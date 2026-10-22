//go:build linux

package runtime

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"
)

func (e *Engine) Run(id string, opts SpawnOptions) error {
	exe, err := os.Executable()
	if err != nil {
		return err
	}

	c := exec.Command(exe, "child", id, opts.Command, e.RootfsPath, opts.Intent)

	flags := uintptr(syscall.CLONE_NEWUTS)
	namespaces := []string{"NEWUTS (Hostname)"}
	
	if opts.NewNS {
		flags |= syscall.CLONE_NEWNS
		namespaces = append(namespaces, "NEWNS (Mounts)")
	}
	if opts.NewPID {
		flags |= syscall.CLONE_NEWPID
		namespaces = append(namespaces, "NEWPID (Process)")
	}
	if opts.NewNet {
		flags |= syscall.CLONE_NEWNET
		namespaces = append(namespaces, "NEWNET (Airgapped)")
	}
	if opts.NewIPC {
		flags |= syscall.CLONE_NEWIPC
		namespaces = append(namespaces, "NEWIPC (Isolated)")
	}

	c.SysProcAttr = &syscall.SysProcAttr{
		Cloneflags: flags,
	}
	if opts.NewNS {
		c.SysProcAttr.Unshareflags = syscall.CLONE_NEWNS
	}

	os.MkdirAll("logs", 0755)
	logFile, _ := os.Create(filepath.Join("logs", id+".log"))
	c.Stdout = io.MultiWriter(os.Stdout, logFile)
	c.Stderr = io.MultiWriter(os.Stderr, logFile)

	if err := c.Start(); err != nil {
		return err
	}

	profile := CompileIntent(opts.Intent)
	
	e.mu.Lock()
	e.Containers[id] = &Container{
		ID:             id,
		PID:            c.Process.Pid,
		Status:         "Running",
		Options:        opts,
		Profile:        profile,
		Namespaces:     namespaces,
		IsolationScore: 100,
		SeccompRules:   14, // Number of blocked syscalls
		StartTime:      time.Now(),
	}
	e.addAuditLocked("INFO", "SYSTEM", id, fmt.Sprintf("Container spawned with namespaces: %v", namespaces))
	e.mu.Unlock()

	go func() {
		c.Wait()
		e.mu.Lock()
		if cont, ok := e.Containers[id]; ok {
			cont.Status = "Stopped"
		}
		e.mu.Unlock()
	}()

	return nil
}

func Child(id, cmd, rootfs, intent string) error {
	fmt.Printf("[Container %s] Intent: '%s'\n", id, intent)
	profile := CompileIntent(intent)
	
	setupCgroups(id, profile)
	setupMounts(rootfs)
	syscall.Sethostname([]byte("aether-" + id[:6]))
	
	// Apply Seccomp-BPF filter BEFORE executing the user command.
	// This blocks dangerous syscalls (reboot, kexec_load, etc.)
	ruleCount, err := ApplySeccompFilter()
	if err != nil {
		fmt.Printf("[Container %s] WARNING: Seccomp filter failed: %v\n", id, err)
	} else {
		fmt.Printf("[Container %s] Seccomp-BPF active: %d syscalls blocked\n", id, ruleCount)
	}
	
	execCmd := exec.Command("/bin/sh", "-c", cmd)
	execCmd.Stdin = os.Stdin
	execCmd.Stdout = os.Stdout
	execCmd.Stderr = os.Stderr
	return execCmd.Run()
}

func setupCgroups(id string, profile SecurityProfile) {
	cgroupsPath := filepath.Join("/sys/fs/cgroup", "aether", id)
	os.MkdirAll(cgroupsPath, 0755)
	
	os.WriteFile(filepath.Join(cgroupsPath, "pids.max"), []byte(fmt.Sprintf("%d", profile.MaxPIDs)), 0700)
	os.WriteFile(filepath.Join(cgroupsPath, "memory.max"), []byte(profile.MemoryMax), 0700)
	os.WriteFile(filepath.Join(cgroupsPath, "cpu.max"), []byte(profile.CPUMax), 0700)
	
	os.WriteFile(filepath.Join(cgroupsPath, "cgroup.procs"), []byte(fmt.Sprintf("%d", os.Getpid())), 0700)
}

func freezeCgroup(id string, freeze bool) error {
	path := filepath.Join("/sys/fs/cgroup", "aether", id, "cgroup.freeze")
	val := "0"
	if freeze {
		val = "1"
	}
	return os.WriteFile(path, []byte(val), 0700)
}

func setupMounts(rootfs string) {
	syscall.Mount("", "/", "", syscall.MS_PRIVATE|syscall.MS_REC, "")
	syscall.Mount(rootfs, rootfs, "bind", syscall.MS_BIND|syscall.MS_REC, "")
	os.MkdirAll(filepath.Join(rootfs, "proc"), 0755)
	syscall.Mount("proc", filepath.Join(rootfs, "proc"), "proc", 0, "")
	syscall.Chroot(rootfs)
	os.Chdir("/")
}

// updateMetrics reads live cgroup telemetry for all running containers.
// Called with mutex held.
func (e *Engine) updateMetrics() {
	for _, c := range e.Containers {
		if c.Status != "Running" {
			continue
		}
		
		memPath := filepath.Join("/sys/fs/cgroup", "aether", c.ID, "memory.current")
		memData, err := os.ReadFile(memPath)
		if err == nil {
			memStr := strings.TrimSpace(string(memData))
			memUsage, _ := strconv.ParseInt(memStr, 10, 64)
			c.Metrics.MemoryUsage = memUsage
			
			c.Metrics.MemoryHistory = append(c.Metrics.MemoryHistory, memUsage)
			if len(c.Metrics.MemoryHistory) > 3 {
				c.Metrics.MemoryHistory = c.Metrics.MemoryHistory[1:]
			}
			
			if len(c.Metrics.MemoryHistory) == 3 {
				c.Metrics.MemoryVelocity = c.Metrics.MemoryHistory[2] - c.Metrics.MemoryHistory[0]
			}
		}

		cpuPath := filepath.Join("/sys/fs/cgroup", "aether", c.ID, "cpu.stat")
		cpuData, err := os.ReadFile(cpuPath)
		if err == nil {
			lines := strings.Split(string(cpuData), "\n")
			for _, line := range lines {
				if strings.HasPrefix(line, "usage_usec") {
					parts := strings.Fields(line)
					if len(parts) >= 2 {
						usec, _ := strconv.ParseFloat(parts[1], 64)
						c.Metrics.CPUUsage = usec / 1000000.0
					}
				}
			}
		}
	}
}

// evaluateAdaptations implements the Predictive Governor.
// It calculates memory velocity (first derivative) and applies
// pre-emptive throttling or elastic scaling based on intent trust level.
// Called with mutex held.
func (e *Engine) evaluateAdaptations() {
	for _, c := range e.Containers {
		if c.Status != "Running" {
			continue
		}
		
		memLimit, _ := strconv.ParseInt(c.Profile.MemoryMax, 10, 64)
		if memLimit == 0 {
			continue
		}
		
		// Predictive Intelligence: first-derivative velocity detection
		if c.Metrics.MemoryVelocity > (10 * 1024 * 1024) { // +10MB over 2 seconds
			if strings.Contains(c.Options.Intent, "untrusted") && c.IsolationScore > 60 {
				c.IsolationScore -= 30
				e.addEventLocked(fmt.Sprintf("PREDICTIVE ALERT: Untrusted %s growing fast (+%d MB/s). Pre-emptively throttling CPU.", c.ID[:8], c.Metrics.MemoryVelocity/1024/1024/2))
				e.addAuditLocked("ALERT", "GOVERNOR", c.ID, fmt.Sprintf("Predictive throttle: velocity=%d MB/s, score=%d", c.Metrics.MemoryVelocity/1024/1024/2, c.IsolationScore))
				
				cgroupsPath := filepath.Join("/sys/fs/cgroup", "aether", c.ID)
				os.WriteFile(filepath.Join(cgroupsPath, "cpu.max"), []byte("1000 100000"), 0700) // 1% CPU
				c.Profile.CPUMax = "1000 100000"
				
				c.Metrics.MemoryHistory = []int64{}
				c.Metrics.MemoryVelocity = 0
			} else if strings.Contains(c.Options.Intent, "batch") && c.IsolationScore >= 90 {
				c.IsolationScore += 5
				if c.IsolationScore > 110 { c.IsolationScore = 110 }
				e.addEventLocked(fmt.Sprintf("PREDICTIVE SCALING: Batch %s velocity high. Pre-allocating +100MB.", c.ID[:8]))
				e.addAuditLocked("INFO", "GOVERNOR", c.ID, "Elastic scaling: +100MB pre-allocated")
				
				newLimit := memLimit + (100 * 1024 * 1024)
				cgroupsPath := filepath.Join("/sys/fs/cgroup", "aether", c.ID)
				newLimitStr := fmt.Sprintf("%d", newLimit)
				os.WriteFile(filepath.Join(cgroupsPath, "memory.max"), []byte(newLimitStr), 0700)
				c.Profile.MemoryMax = newLimitStr
				
				c.Metrics.MemoryHistory = []int64{}
				c.Metrics.MemoryVelocity = 0
			}
		}
		
		usageRatio := float64(c.Metrics.MemoryUsage) / float64(memLimit)
		
		// Reactive Intelligence: threshold-based response
		if usageRatio > 0.75 && c.IsolationScore >= 50 {
			if strings.Contains(c.Options.Intent, "untrusted") && c.IsolationScore > 80 {
				c.IsolationScore -= 25
				e.addEventLocked(fmt.Sprintf("INTENT ALERT: Untrusted container %s spiking memory (%.0f%%). Restricting CPU. Score -> %d", c.ID[:8], usageRatio*100, c.IsolationScore))
				e.addAuditLocked("WARN", "GOVERNOR", c.ID, fmt.Sprintf("Reactive throttle: usage=%.0f%%, score=%d", usageRatio*100, c.IsolationScore))
				
				cgroupsPath := filepath.Join("/sys/fs/cgroup", "aether", c.ID)
				os.WriteFile(filepath.Join(cgroupsPath, "cpu.max"), []byte("5000 100000"), 0700)
				c.Profile.CPUMax = "5000 100000"
				
			} else if strings.Contains(c.Options.Intent, "batch") && c.IsolationScore == 100 {
				c.IsolationScore += 10
				if c.IsolationScore > 110 { c.IsolationScore = 110 }
				e.addEventLocked(fmt.Sprintf("ADAPTATION: Trusted batch %s needs memory. Relaxing bounds elastically (+50MB).", c.ID[:8]))
				e.addAuditLocked("INFO", "GOVERNOR", c.ID, "Elastic scaling: +50MB allocated")
				
				newLimit := memLimit + (50 * 1024 * 1024)
				cgroupsPath := filepath.Join("/sys/fs/cgroup", "aether", c.ID)
				newLimitStr := fmt.Sprintf("%d", newLimit)
				os.WriteFile(filepath.Join(cgroupsPath, "memory.max"), []byte(newLimitStr), 0700)
				c.Profile.MemoryMax = newLimitStr
			}
		}
	}
}
