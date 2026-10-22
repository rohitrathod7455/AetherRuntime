package controlplane

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os/exec"
	"strings"
	"time"

	"aether-runtime/runtime"
)

type Server struct {
	engine *runtime.Engine
}

func NewServer(engine *runtime.Engine) *Server {
	return &Server{engine: engine}
}

func (s *Server) Start(port int) error {
	mux := http.NewServeMux()
	
	mux.HandleFunc("/api/containers", s.handleContainers)
	mux.HandleFunc("/api/containers/", s.handleContainerAction)
	mux.HandleFunc("/api/autopilot", s.handleAutopilot)
	mux.HandleFunc("/api/audit", s.handleAuditLog)
	mux.HandleFunc("/api/events", s.handleEvents)

	handler := corsMiddleware(mux)

	addr := fmt.Sprintf(":%d", port)
	fmt.Printf("[ControlPlane] API listening on %s\n", addr)
	return http.ListenAndServe(addr, handler)
}

func corsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
		if r.Method == "OPTIONS" {
			w.WriteHeader(http.StatusOK)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (s *Server) handleContainers(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet {
		containers := s.engine.ListContainers()
		json.NewEncoder(w).Encode(containers)
	} else if r.Method == http.MethodPost {
		var req runtime.SpawnOptions
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		
		if req.Intent == "" {
			req.Intent = "default"
		}
		
		id := fmt.Sprintf("c-%d", time.Now().UnixNano()%1000000)
		if err := s.engine.Run(id, req); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(map[string]string{"id": id, "status": "Started"})
	}
}

func minInt(a, b int) int {
	if a < b { return a }
	return b
}

// handleAutopilot uses the Naive Bayes classifier to infer intent from raw commands.
func (s *Server) handleAutopilot(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req struct { Command string `json:"command"` }
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	// --- ML Classification ---
	result := s.engine.Classifier.Classify(req.Command)
	intent := string(result.PredictedIntent)
	confidence := result.Confidence

	// Map classified intent to namespace configuration
	opts := runtime.SpawnOptions{
		Command: "echo 'Autopilot Sandbox' && sleep 60",
		Intent:  intent,
		NewNS:   true,
		NewPID:  true,
		NewIPC:  true,
		NewNet:  true, // Default: airgapped
	}

	// Only open network for network-classified tasks
	if result.PredictedIntent == runtime.IntentNetwork {
		opts.NewNet = false
	}
	// Dangerous commands get maximum isolation
	if result.PredictedIntent == runtime.IntentDangerous {
		opts.NewNet = true
		opts.NewIPC = true
	}

	id := fmt.Sprintf("auto-%d", time.Now().UnixNano()%100000)
	s.engine.Run(id, opts)

	cmdPreview := req.Command[:minInt(25, len(req.Command))]
	s.engine.AddEvent(fmt.Sprintf("ML-ENGINE: Classified '%s...' → %s (%.1f%% confidence). Spawning %s.",
		cmdPreview, intent, confidence*100, id))

	time.Sleep(300 * time.Millisecond) // Allow container to initialize

	var output string
	if c, ok := s.engine.GetContainer(id); ok {
		args := []string{"-t", fmt.Sprintf("%d", c.PID), "-a", "/bin/sh", "-c", req.Command}
		cmd := exec.Command("nsenter", args...)
		out, err := cmd.CombinedOutput()
		output = string(out)
		if err != nil {
			output += "\n[Error: " + err.Error() + "]"
		}
	}

	c, _ := s.engine.GetContainer(id)
	var namespaces []string
	if c != nil {
		namespaces = c.Namespaces
	}

	json.NewEncoder(w).Encode(map[string]interface{}{
		"id":             id,
		"intent":         intent,
		"confidence":     confidence,
		"probabilities":  result.Probabilities,
		"namespaces":     namespaces,
		"output":         output,
	})
}

func (s *Server) handleContainerAction(w http.ResponseWriter, r *http.Request) {
	parts := strings.Split(r.URL.Path, "/")
	if len(parts) < 4 {
		http.Error(w, "Invalid route", http.StatusBadRequest)
		return
	}
	id := parts[3]

	if r.Method == http.MethodDelete {
		s.engine.Stop(id)
		w.WriteHeader(http.StatusOK)
	} else if r.Method == http.MethodPost {
		if strings.HasSuffix(r.URL.Path, "/exec") {
			var req struct { Command string `json:"command"` }
			if err := json.NewDecoder(r.Body).Decode(&req); err == nil {
				c, ok := s.engine.GetContainer(id)
				if !ok { http.Error(w, "not found", 404); return }
				
				args := []string{"-t", fmt.Sprintf("%d", c.PID), "-a", "/bin/sh", "-c", req.Command}
				cmd := exec.Command("nsenter", args...)
				out, err := cmd.CombinedOutput()
				
				resp := string(out)
				if err != nil {
					resp += "\n[Error: " + err.Error() + "]"
				}
				json.NewEncoder(w).Encode(map[string]string{"output": resp})
				return
			}
		} else {
			var req struct {
				Action string `json:"action"`
			}
			if err := json.NewDecoder(r.Body).Decode(&req); err == nil {
				if req.Action == "freeze" {
					s.engine.Freeze(id)
				} else if req.Action == "thaw" {
					s.engine.Thaw(id)
				}
			}
			w.WriteHeader(http.StatusOK)
		}
	} else {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

// handleAuditLog exposes the structured audit trail for research/forensics.
func (s *Server) handleAuditLog(w http.ResponseWriter, r *http.Request) {
	logs := s.engine.GetAuditLogSnapshot()
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(logs)
}

func (s *Server) handleEvents(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("Access-Control-Allow-Origin", "*")

	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "Streaming unsupported", http.StatusInternalServerError)
		return
	}

	fmt.Fprintf(w, "data: connected\n\n")
	flusher.Flush()

	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-r.Context().Done():
			return
		case <-ticker.C:
			containers := s.engine.ListContainers()
			events := s.engine.GetEventsSnapshot()
			
			payload := map[string]interface{}{
				"containers": containers,
				"events":     events,
			}
			
			data, err := json.Marshal(payload)
			if err == nil {
				fmt.Fprintf(w, "data: %s\n\n", string(data))
				flusher.Flush()
			}
		}
	}
}
