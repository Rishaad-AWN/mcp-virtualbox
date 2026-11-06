//go:build !windows

package vboxweb

import (
	"os/exec"
	"syscall"
)

// detach puts the child in its own process group so it is not killed when the
// MCP server (this process) exits or receives a signal.
func detach(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
}
