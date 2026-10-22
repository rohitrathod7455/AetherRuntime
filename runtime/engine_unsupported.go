//go:build !linux

package runtime

import "fmt"

func (e *Engine) Run(id string, cmd string, intent string) error {
	return fmt.Errorf("isolation is only supported on Linux")
}

func Child(id, cmd, rootfs, intent string) error {
	return fmt.Errorf("isolation is only supported on Linux")
}

func freezeCgroup(id string, freeze bool) error {
	return nil
}

func (e *Engine) updateMetrics() {
	// No-op for Windows/macOS fallback
}
