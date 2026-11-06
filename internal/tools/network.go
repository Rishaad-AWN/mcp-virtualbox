package tools

import (
	"bytes"
	"context"
	"fmt"
	"net"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/masterzen/winrm"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"golang.org/x/crypto/ssh"

	"github.com/Rishaad-AWN/mcp-virtualbox/internal/vbox"
)

var ipRe = regexp.MustCompile(`value:\s+(\d+\.\d+\.\d+\.\d+)`)

func registerNetwork(s *mcp.Server, d *deps) {
	add(s, "vm_exec_ssh", "Execute a command on a Linux guest via SSH. The guest's host key is trusted on first "+
		"connect and checked on every later one; a changed key is refused (set acceptNewHostKey=true only "+
		"after reinstalling/recreating the VM).",
		object([]string{"username", "command"},
			p("host", def(str("SSH host (use 127.0.0.1 with port forwarding)"), "127.0.0.1")),
			p("port", def(integer("SSH port"), 2222)),
			p("username", str("SSH username")),
			p("password", str("SSH password")),
			p("privateKey", str("SSH private key contents")),
			p("command", str("Command to execute")),
			p("timeoutMs", def(integer("Timeout in ms"), 30000)),
			p("acceptNewHostKey", def(boolean("Replace a remembered host key that no longer matches (only after the VM was reinstalled or replaced)"), false)),
		),
		func(ctx context.Context, in struct {
			AcceptNewHostKey bool   `json:"acceptNewHostKey"`
			Host             string `json:"host"`
			Port             int    `json:"port"`
			Username         string `json:"username"`
			Password         string `json:"password"`
			PrivateKey       string `json:"privateKey"`
			Command          string `json:"command"`
			TimeoutMs        int    `json:"timeoutMs"`
		}) (*mcp.CallToolResult, error) {
			return execSSH(ctx, in.Host, in.Port, in.Username, in.Password, in.PrivateKey, in.Command, in.TimeoutMs, in.AcceptNewHostKey)
		})

	add(s, "vm_exec_winrm", "Execute a command on a Windows guest via WinRM (SOAP/HTTP, Basic auth)",
		object([]string{"username", "password", "command"},
			p("host", def(str("WinRM host"), "127.0.0.1")),
			p("port", def(integer("WinRM HTTP port"), 5985)),
			p("username", str("Windows username")),
			p("password", str("Windows password")),
			p("command", str("Command to execute (cmd.exe)")),
			p("usePowershell", def(boolean("Execute as PowerShell command"), false)),
			p("timeoutMs", def(integer("Timeout in ms"), 30000)),
		),
		func(ctx context.Context, in struct {
			Host          string `json:"host"`
			Port          int    `json:"port"`
			Username      string `json:"username"`
			Password      string `json:"password"`
			Command       string `json:"command"`
			UsePowershell bool   `json:"usePowershell"`
			TimeoutMs     int    `json:"timeoutMs"`
		}) (*mcp.CallToolResult, error) {
			return execWinRM(ctx, in.Host, in.Port, in.Username, in.Password, in.Command, in.UsePowershell, in.TimeoutMs)
		})

	add(s, "vm_get_guest_ip",
		"Get the guest's IP addresses. Uses Guest Additions when present; otherwise matches the VM's NIC MAC "+
			"addresses against VirtualBox's DHCP leases (host-only/NAT network) and the host ARP table, so "+
			"routers and appliances without Guest Additions are found too. Each result says its source. "+
			"NAT (nic type \"nat\") guests are not reachable by IP from the host: use vm_add_nat_port_forward.",
		object([]string{"vm"}, p("vm", str("VM name or UUID"))),
		func(ctx context.Context, in struct {
			VM string `json:"vm"`
		}) (*mcp.CallToolResult, error) {
			ips, err := d.findGuestIPs(ctx, in.VM)
			if err != nil {
				return errText("Failed to get guest IP: " + vbox.FormatError(err)), nil
			}
			if len(ips) == 0 {
				return text("No guest IP found: no Guest Additions, no DHCP lease and no ARP entry for this VM's MACs. " +
					"The VM may still be booting, use a static IP the host has not talked to yet, or only have NAT networking."), nil
			}
			return jsonText(ips), nil
		})

	add(s, "vm_wait_for_guest_additions", "Wait until Guest Additions are running (polls GuestAdditionsRunLevel)",
		object([]string{"vm"},
			p("vm", str("VM name or UUID")),
			p("timeoutMs", def(integer("Timeout in ms"), 120000)),
			p("pollIntervalMs", def(integer("Poll interval in ms"), 3000)),
		),
		func(ctx context.Context, in struct {
			VM             string `json:"vm"`
			TimeoutMs      int    `json:"timeoutMs"`
			PollIntervalMs int    `json:"pollIntervalMs"`
		}) (*mcp.CallToolResult, error) {
			ok := poll(ctx, in.TimeoutMs, in.PollIntervalMs, func() (string, bool) {
				out, err := d.vb.Run(ctx, "showvminfo", in.VM, "--machinereadable")
				if err != nil {
					return "", false
				}
				level, _ := strconv.Atoi(vbox.ParseMachineReadable(out)["GuestAdditionsRunLevel"])
				if level >= 2 {
					return fmt.Sprintf("Guest Additions running (level %d).", level), true
				}
				return "", false
			})
			if ok != "" {
				return text(ok), nil
			}
			return errText(fmt.Sprintf("Timeout waiting for Guest Additions after %dms.", in.TimeoutMs)), nil
		})

	add(s, "vm_wait_for_network", "Wait until the guest has an IP address (same sources as vm_get_guest_ip)",
		object([]string{"vm"},
			p("vm", str("VM name or UUID")),
			p("timeoutMs", def(integer("Timeout in ms"), 120000)),
			p("pollIntervalMs", def(integer("Poll interval in ms"), 3000)),
		),
		func(ctx context.Context, in struct {
			VM             string `json:"vm"`
			TimeoutMs      int    `json:"timeoutMs"`
			PollIntervalMs int    `json:"pollIntervalMs"`
		}) (*mcp.CallToolResult, error) {
			ok := poll(ctx, in.TimeoutMs, in.PollIntervalMs, func() (string, bool) {
				ips, err := d.findGuestIPs(ctx, in.VM)
				if err != nil || len(ips) == 0 {
					return "", false
				}
				return fmt.Sprintf("Network ready. Guest IP: %s (from %s)", ips[0].IP, ips[0].Source), true
			})
			if ok != "" {
				return text(ok), nil
			}
			return errText(fmt.Sprintf("Timeout waiting for network after %dms.", in.TimeoutMs)), nil
		})

	add(s, "vm_add_nat_port_forward", "Add a NAT port forwarding rule to the VM",
		object([]string{"vm", "name", "hostPort", "guestPort"},
			p("vm", str("VM name or UUID")),
			p("name", str("Rule name (e.g. 'ssh', 'winrm')")),
			p("protocol", def(enum("", "tcp", "udp"), "tcp")),
			p("hostPort", integer("Host port")),
			p("guestPort", integer("Guest port")),
			p("adapter", def(integer("Network adapter number (1-based)"), 1)),
		),
		func(ctx context.Context, in struct {
			VM        string `json:"vm"`
			Name      string `json:"name"`
			Protocol  string `json:"protocol"`
			HostPort  int    `json:"hostPort"`
			GuestPort int    `json:"guestPort"`
			Adapter   int    `json:"adapter"`
		}) (*mcp.CallToolResult, error) {
			rule := fmt.Sprintf("%s,%s,,%d,,%d", in.Name, in.Protocol, in.HostPort, in.GuestPort)
			// Running/paused VMs need controlvm; powered-off VMs need modifyvm.
			info, err := d.vb.Run(ctx, "showvminfo", in.VM, "--machinereadable")
			if err != nil {
				return fail(err), nil
			}
			state := vbox.ParseMachineReadable(info)["VMState"]
			if state == "running" || state == "paused" {
				_, err = d.vb.Run(ctx, "controlvm", in.VM, fmt.Sprintf("natpf%d", in.Adapter), rule)
			} else {
				_, err = d.vb.Run(ctx, "modifyvm", in.VM, fmt.Sprintf("--natpf%d", in.Adapter), rule)
			}
			if err != nil {
				return fail(err), nil
			}
			return text(fmt.Sprintf("NAT port forward added: %s %s host:%d -> guest:%d", in.Name, in.Protocol, in.HostPort, in.GuestPort)), nil
		})
}

func extractIPs(output string) []string {
	var ips []string
	for _, line := range strings.Split(output, "\n") {
		if m := ipRe.FindStringSubmatch(line); m != nil && m[1] != "0.0.0.0" {
			ips = append(ips, m[1])
		}
	}
	return ips
}

// poll runs check every intervalMs until it succeeds or timeoutMs elapses,
// returning the success message (or "" on timeout). It also aborts if ctx is
// cancelled.
func poll(ctx context.Context, timeoutMs, intervalMs int, check func() (string, bool)) string {
	deadline := time.Now().Add(time.Duration(timeoutMs) * time.Millisecond)
	interval := time.Duration(intervalMs) * time.Millisecond
	for time.Now().Before(deadline) {
		if msg, ok := check(); ok {
			return msg
		}
		select {
		case <-ctx.Done():
			return ""
		case <-time.After(interval):
		}
	}
	return ""
}

func execSSH(ctx context.Context, host string, port int, username, password, privateKey, command string, timeoutMs int, acceptNewHostKey bool) (*mcp.CallToolResult, error) {
	timeout := time.Duration(timeoutMs) * time.Millisecond
	var auth []ssh.AuthMethod
	if password != "" {
		auth = append(auth, ssh.Password(password))
	}
	if privateKey != "" {
		signer, err := ssh.ParsePrivateKey([]byte(privateKey))
		if err != nil {
			return errText("SSH private key parse error: " + err.Error()), nil
		}
		auth = append(auth, ssh.PublicKeys(signer))
	}

	cfg := &ssh.ClientConfig{
		User:            username,
		Auth:            auth,
		HostKeyCallback: tofuHostKey(knownHostsPath(), acceptNewHostKey),
		Timeout:         timeout,
	}
	addr := net.JoinHostPort(host, strconv.Itoa(port))
	client, err := ssh.Dial("tcp", addr, cfg)
	if err != nil {
		return errText("SSH connection error: " + err.Error()), nil
	}
	defer client.Close()

	session, err := client.NewSession()
	if err != nil {
		return errText("SSH exec error: " + err.Error()), nil
	}
	defer session.Close()

	var stdout, stderr bytes.Buffer
	session.Stdout = &stdout
	session.Stderr = &stderr

	type result struct {
		code int
		err  error
	}
	done := make(chan result, 1)
	go func() {
		runErr := session.Run(command)
		code := 0
		if ee, ok := runErr.(*ssh.ExitError); ok {
			code = ee.ExitStatus()
			runErr = nil
		}
		done <- result{code: code, err: runErr}
	}()

	select {
	case <-time.After(timeout):
		return errText(fmt.Sprintf("SSH timeout after %dms.\nstdout: %s\nstderr: %s", timeoutMs, stdout.String(), stderr.String())), nil
	case r := <-done:
		if r.err != nil {
			return errText("SSH exec error: " + r.err.Error()), nil
		}
		parts := []string{fmt.Sprintf("Exit code: %d", r.code)}
		if stdout.Len() > 0 {
			parts = append(parts, "stdout:\n"+stdout.String())
		}
		if stderr.Len() > 0 {
			parts = append(parts, "stderr:\n"+stderr.String())
		}
		return text(strings.Join(parts, "\n")), nil
	}
}

func execWinRM(ctx context.Context, host string, port int, username, password, command string, usePowershell bool, timeoutMs int) (*mcp.CallToolResult, error) {
	timeout := time.Duration(timeoutMs) * time.Millisecond
	endpoint := winrm.NewEndpoint(host, port, false, false, nil, nil, nil, timeout)
	client, err := winrm.NewClient(endpoint, username, password)
	if err != nil {
		return errText("WinRM client error: " + err.Error()), nil
	}

	cmd := command
	if usePowershell {
		cmd = winrm.Powershell(command)
	}

	runCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	stdout, stderr, _, err := client.RunWithContextWithString(runCtx, cmd, "")
	if err != nil {
		return errText("WinRM exec error: " + err.Error()), nil
	}

	var parts []string
	if stdout != "" {
		parts = append(parts, "stdout:\n"+stdout)
	}
	if stderr != "" {
		parts = append(parts, "stderr:\n"+stderr)
	}
	if len(parts) == 0 {
		return text("(no output)"), nil
	}
	return text(strings.Join(parts, "\n")), nil
}
