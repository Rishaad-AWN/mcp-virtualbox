//go:build !windows

package hostnet

import "os/exec"

func hideWindow(*exec.Cmd) {}
