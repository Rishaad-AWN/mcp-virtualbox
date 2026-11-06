//go:build windows

package vboxweb

import (
	"os/exec"
	"syscall"
)

// createNewProcessGroup detaches the child from our console so it keeps running
// after the MCP server exits.
const createNewProcessGroup = 0x00000200 // CREATE_NEW_PROCESS_GROUP

func detach(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{CreationFlags: createNewProcessGroup}
}
