package runtime

import "strings"

type SecurityProfile struct {
	MaxPIDs   int    `json:"max_pids"`
	MemoryMax string `json:"memory_max"`
	CPUMax    string `json:"cpu_max"`
}

// CompileIntent maps a semantic intent string to kernel-enforced security limits.
// This is the "Intent Compiler" — it translates human-readable descriptions
// into concrete cgroup v2 parameters.
func CompileIntent(intent string) SecurityProfile {
	intent = strings.ToLower(intent)

	profile := SecurityProfile{
		MaxPIDs:   50,
		MemoryMax: "200000000",    // ~200MB
		CPUMax:    "50000 100000", // 50% CPU
	}

	switch {
	case strings.Contains(intent, "untrusted") || strings.Contains(intent, "dangerous"):
		profile.MaxPIDs = 10
		profile.MemoryMax = "50000000"    // 50MB
		profile.CPUMax = "10000 100000"   // 10% CPU
	case strings.Contains(intent, "network"):
		profile.MaxPIDs = 30
		profile.MemoryMax = "100000000"   // 100MB
		profile.CPUMax = "25000 100000"   // 25% CPU
	case strings.Contains(intent, "compute"):
		profile.MaxPIDs = 100
		profile.MemoryMax = "500000000"   // 500MB
		profile.CPUMax = "80000 100000"   // 80% CPU
	case strings.Contains(intent, "file"):
		profile.MaxPIDs = 30
		profile.MemoryMax = "150000000"   // 150MB
		profile.CPUMax = "30000 100000"   // 30% CPU
	case strings.Contains(intent, "batch"):
		profile.MaxPIDs = 100
		profile.MemoryMax = "1000000000"  // 1GB
		profile.CPUMax = "100000 100000"  // 100% CPU
	case strings.Contains(intent, "training"):
		profile.MaxPIDs = 200
		profile.MemoryMax = "4000000000"  // 4GB
		profile.CPUMax = "max 100000"
	}

	return profile
}
