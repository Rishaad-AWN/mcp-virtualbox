package tools

import (
	"context"
	"fmt"
	"os"
	"strconv"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/Rishaad-AWN/mcp-virtualbox/internal/vbox"
)

// secretFile writes secret to a private temp file for VBoxManage's
// --passwordfile/--password-file flags, so it never appears in the process list.
// Call cleanup when VBoxManage is done.
func secretFile(secret string) (path string, cleanup func(), err error) {
	f, err := os.CreateTemp("", "vbox-secret-*")
	if err != nil {
		return "", nil, fmt.Errorf("could not create password file: %w", err)
	}
	cleanup = func() { os.Remove(f.Name()) }
	_, err = f.WriteString(secret)
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		cleanup()
		return "", nil, fmt.Errorf("could not write password file: %w", err)
	}
	return f.Name(), cleanup, nil
}

// credentialArgs builds the shared --username/--passwordfile flags. A blank
// password is omitted since VBoxManage treats the flag's
// absence as "no password".
func credentialArgs(username, password string) (args []string, cleanup func(), err error) {
	args, cleanup = []string{"--username", username}, func() {}
	if password != "" {
		var path string
		if path, cleanup, err = secretFile(password); err != nil {
			return nil, nil, err
		}
		args = append(args, "--passwordfile="+path)
	}
	return args, cleanup, nil
}

func registerGuest(s *mcp.Server, d *deps) {
	add(s, "vm_guest_exec",
		"Execute a command inside the guest via Guest Additions (no network required). Guest Additions "+
			"must be installed and running.",
		object([]string{"vm", "executable", "username"},
			p("vm", str("VM name or UUID")),
			p("executable", str(`Path to executable inside guest. For Windows commands, use C:\WINDOWS\system32\cmd.exe with args ["/c", "command", "arg1", "arg2"]`)),
			p("args", def(array(`Command arguments. IMPORTANT: Each argument must be a SEPARATE array element. Do NOT combine command+args into one string. Example: ["/c", "dir", "C:\\"] not ["/c", "dir C:\\"]. Also: VBoxManage guestcontrol breaks arguments containing spaces - avoid paths with spaces in args. For complex commands, use vm_guest_copy_to to write a .bat/.sh script, then execute it.`, str("")), []string{})),
			p("username", str("Guest OS username")),
			p("password", def(str("Guest OS password. IMPORTANT: Windows accounts with BLANK passwords CANNOT use guestcontrol - you must set a password first (use vision loop: Win+R -> cmd -> 'net user <name> <pass>')"), "")),
			p("timeoutMs", def(integer("Timeout in ms"), 30000)),
		),
		func(ctx context.Context, in struct {
			VM         string   `json:"vm"`
			Executable string   `json:"executable"`
			Args       []string `json:"args"`
			Username   string   `json:"username"`
			Password   string   `json:"password"`
			TimeoutMs  int      `json:"timeoutMs"`
		}) (*mcp.CallToolResult, error) {
			creds, cleanup, err := credentialArgs(in.Username, in.Password)
			if err != nil {
				return errText(err.Error()), nil
			}
			defer cleanup()
			vboxArgs := []string{"guestcontrol", in.VM, "run", "--exe", in.Executable}
			vboxArgs = append(vboxArgs, creds...)
			vboxArgs = append(vboxArgs, "--wait-stdout", "--wait-stderr", "--timeout", strconv.Itoa(in.TimeoutMs))
			if len(in.Args) > 0 {
				vboxArgs = append(vboxArgs, "--")
				vboxArgs = append(vboxArgs, in.Args...)
			}
			out, err := d.vb.Run(ctx, vboxArgs...)
			if err != nil {
				return errText("Guest exec failed: " + vbox.FormatError(err)), nil
			}
			if out == "" {
				out = "(no output)"
			}
			return text(out), nil
		})

	add(s, "vm_guest_copy_to", "Copy a file from host to guest via Guest Additions",
		object([]string{"vm", "hostPath", "guestPath", "username"},
			p("vm", str("VM name or UUID")),
			p("hostPath", str("Path on host")),
			p("guestPath", str("Path on guest")),
			p("username", str("Guest OS username")),
			p("password", def(str("Guest OS password"), "")),
		),
		func(ctx context.Context, in struct {
			VM        string `json:"vm"`
			HostPath  string `json:"hostPath"`
			GuestPath string `json:"guestPath"`
			Username  string `json:"username"`
			Password  string `json:"password"`
		}) (*mcp.CallToolResult, error) {
			creds, cleanup, err := credentialArgs(in.Username, in.Password)
			if err != nil {
				return errText(err.Error()), nil
			}
			defer cleanup()
			args := []string{"guestcontrol", in.VM, "copyto", "--target-directory", in.GuestPath}
			args = append(args, creds...)
			args = append(args, in.HostPath)
			if _, err := d.vb.Run(ctx, args...); err != nil {
				return fail(err), nil
			}
			return text(fmt.Sprintf("Copied %q -> guest:%q", in.HostPath, in.GuestPath)), nil
		})

	add(s, "vm_guest_copy_from", "Copy a file from guest to host via Guest Additions",
		object([]string{"vm", "guestPath", "hostPath", "username"},
			p("vm", str("VM name or UUID")),
			p("guestPath", str("Path on guest")),
			p("hostPath", str("Path on host")),
			p("username", str("Guest OS username")),
			p("password", def(str("Guest OS password"), "")),
		),
		func(ctx context.Context, in struct {
			VM        string `json:"vm"`
			GuestPath string `json:"guestPath"`
			HostPath  string `json:"hostPath"`
			Username  string `json:"username"`
			Password  string `json:"password"`
		}) (*mcp.CallToolResult, error) {
			creds, cleanup, err := credentialArgs(in.Username, in.Password)
			if err != nil {
				return errText(err.Error()), nil
			}
			defer cleanup()
			args := []string{"guestcontrol", in.VM, "copyfrom", "--target-directory", in.HostPath}
			args = append(args, creds...)
			args = append(args, in.GuestPath)
			if _, err := d.vb.Run(ctx, args...); err != nil {
				return fail(err), nil
			}
			return text(fmt.Sprintf("Copied guest:%q -> %q", in.GuestPath, in.HostPath)), nil
		})
}
