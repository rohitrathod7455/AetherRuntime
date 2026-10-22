package runtime

import (
	"os"
	"sync"
	"time"
)

type Metrics struct {
	CPUUsage       float64 `json:"cpu_usage"`
	MemoryUsage    int64   `json:"memory_usage"`
	MemoryHistory  []int64 `json:"-"`
	MemoryVelocity int64   `json:"memory_velocity"`
}

type SpawnOptions struct {
	Command string `json:"command"`
	Intent  string `json:"intent"`
	NewNet  bool   `json:"new_net"`
	NewIPC  bool   `json:"new_ipc"`
	NewPID  bool   `json:"new_pid"`
	NewNS   bool   `json:"new_ns"`
}

type Container struct {
	ID             string          `json:"id"`
	PID            int             `json:"pid"`
	Status         string          `json:"status"`
	Options        SpawnOptions    `json:"options"`
	Profile        SecurityProfile `json:"profile"`
	Namespaces     []string        `json:"namespaces"`
	StartTime      time.Time       `json:"start_time"`
	Metrics        Metrics         `json:"metrics"`
	IsolationScore int             `json:"isolation_score"`
	SeccompRules   int             `json:"seccomp_rules"`
}

// AuditEntry is a structured, timestamped event record for the causal log.
type AuditEntry struct {
	Timestamp  time.Time `json:"timestamp"`
	Level      string    `json:"level"`    // INFO, WARN, ALERT, CRITICAL
	Category   string    `json:"category"` // SYSTEM, ML-ENGINE, GOVERNOR, SECCOMP
	ContainerID string   `json:"container_id,omitempty"`
	Message    string    `json:"message"`
}

type Engine struct {
	mu         sync.RWMutex
	Containers map[string]*Container
	RootfsPath string
	Events     []string
	AuditLog   []AuditEntry
	Classifier *NaiveBayesClassifier
}

func NewEngine(rootfsPath string) *Engine {
	e := &Engine{
		Containers: make(map[string]*Container),
		RootfsPath: rootfsPath,
		Events:     make([]string, 0),
		AuditLog:   make([]AuditEntry, 0),
		Classifier: NewNaiveBayesClassifier(),
	}
	e.AddEvent("SYSTEM: AetherRuntime v2.0 — Daemon initialized. Naive Bayes classifier loaded with " +
		"130+ labeled commands. Seccomp-BPF filter active.")
	e.addAuditLocked("INFO", "SYSTEM", "", "Engine initialized with ML classifier and Seccomp filter")
	go e.TelemetryLoop()
	return e
}

func (e *Engine) addAuditLocked(level, category, containerID, message string) {
	entry := AuditEntry{
		Timestamp:   time.Now(),
		Level:       level,
		Category:    category,
		ContainerID: containerID,
		Message:     message,
	}
	e.AuditLog = append(e.AuditLog, entry)
	if len(e.AuditLog) > 200 {
		e.AuditLog = e.AuditLog[1:]
	}
}

// AddAudit thread-safely logs an audit entry.
func (e *Engine) AddAudit(level, category, containerID, message string) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.addAuditLocked(level, category, containerID, message)
}

func (e *Engine) addEventLocked(msg string) {
	e.Events = append(e.Events, msg)
	if len(e.Events) > 50 {
		e.Events = e.Events[1:]
	}
}

// AddEvent thread-safely appends an event to the events list.
func (e *Engine) AddEvent(msg string) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.addEventLocked(msg)
}

func (e *Engine) TelemetryLoop() {
	for {
		time.Sleep(1 * time.Second)
		e.mu.Lock()
		e.updateMetrics()
		e.evaluateAdaptations()
		e.mu.Unlock()
	}
}

func (e *Engine) Freeze(id string) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	if c, ok := e.Containers[id]; ok {
		c.Status = "Frozen"
		e.addEventLocked("SYSTEM: Container " + id[:8] + " execution frozen at kernel level.")
		e.addAuditLocked("INFO", "SYSTEM", id, "Container frozen via cgroup.freeze")
		return freezeCgroup(id, true)
	}
	return os.ErrNotExist
}

func (e *Engine) Thaw(id string) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	if c, ok := e.Containers[id]; ok {
		c.Status = "Running"
		e.addEventLocked("SYSTEM: Container " + id[:8] + " execution thawed.")
		e.addAuditLocked("INFO", "SYSTEM", id, "Container thawed")
		return freezeCgroup(id, false)
	}
	return os.ErrNotExist
}

func (e *Engine) Stop(id string) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	if c, ok := e.Containers[id]; ok {
		proc, err := os.FindProcess(c.PID)
		if err == nil {
			proc.Kill()
		}
		c.Status = "Stopped"
		e.addAuditLocked("INFO", "SYSTEM", id, "Container terminated")
		delete(e.Containers, id)
	}
	return nil
}

// GetContainer safely retrieves a container by ID.
func (e *Engine) GetContainer(id string) (*Container, bool) {
	e.mu.RLock()
	defer e.mu.RUnlock()
	c, ok := e.Containers[id]
	return c, ok
}

// ListContainers safely returns a snapshot of all containers.
func (e *Engine) ListContainers() []*Container {
	e.mu.RLock()
	defer e.mu.RUnlock()
	list := make([]*Container, 0, len(e.Containers))
	for _, c := range e.Containers {
		list = append(list, c)
	}
	return list
}

// GetEventsSnapshot safely returns current events.
func (e *Engine) GetEventsSnapshot() []string {
	e.mu.RLock()
	defer e.mu.RUnlock()
	snapshot := make([]string, len(e.Events))
	copy(snapshot, e.Events)
	return snapshot
}

// GetAuditLogSnapshot safely returns current audit logs.
func (e *Engine) GetAuditLogSnapshot() []AuditEntry {
	e.mu.RLock()
	defer e.mu.RUnlock()
	snapshot := make([]AuditEntry, len(e.AuditLog))
	copy(snapshot, e.AuditLog)
	return snapshot
}

