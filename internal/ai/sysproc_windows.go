//go:build windows

package ai

import "os/exec"

func setSysProcAttr(cmd *exec.Cmd) {
	// Setsid is not available on Windows; no-op.
}
