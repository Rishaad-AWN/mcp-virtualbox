//go:build windows

package hostnet

import (
	"os/exec"
	"syscall"
)

// hideWindow stops arp.exe flashing a console window.
func hideWindow(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: 0x08000000} // CREATE_NO_WINDOW
}
