package tools

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/Rishaad-AWN/mcp-virtualbox/internal/vbox"
)

func registerFeatures(s *mcp.Server, d *deps) {
	add(s, "vm_logs",
		"Read the end of a VM's VBox.log: the first place to look when a VM fails to start, crashes "+
			"(Guru Meditation), or misbehaves. Optional 'filter' keeps only lines containing that text.",
		object([]string{"vm"},
			p("vm", str("VM name or UUID")),
			p("lines", def(integer("How many lines from the end to return"), 80)),
			p("filter", str("Only lines containing this text, case-insensitive (e.g. 'error', 'NAT', 'ACPI')")),
		),
		func(ctx context.Context, in struct {
			VM     string `json:"vm"`
			Lines  int    `json:"lines"`
			Filter string `json:"filter"`
		}) (*mcp.CallToolResult, error) {
			out, err := d.vb.Run(ctx, "showvminfo", in.VM, "--log", "0")
			if err != nil {
				return fail(err), nil
			}
			return text(tailLines(out, in.Filter, in.Lines)), nil
		})

	add(s, "vm_wait_for_port",
		"Wait until a TCP service answers, e.g. SSH (22) after an install or reboot. By default the service "+
			"must greet first (SSH, telnet, FTP, SMTP do): needed for NAT port forwards, where VirtualBox "+
			"accepts the host side even while the guest is still booting. For HTTP and other services that "+
			"wait for the client, set needsBanner=false (on a NAT forward that only proves the forward exists).",
		object([]string{"port"},
			p("host", def(str("Host or guest IP (127.0.0.1 for a NAT port forward)"), "127.0.0.1")),
			p("port", integer("TCP port")),
			p("timeoutSec", def(integer("Max seconds to wait"), 120)),
			p("needsBanner", def(boolean("Only count it ready once the service sends its greeting"), true)),
		),
		func(ctx context.Context, in struct {
			NeedsBanner bool   `json:"needsBanner"`
			Host        string `json:"host"`
			Port        int    `json:"port"`
			TimeoutSec  int    `json:"timeoutSec"`
		}) (*mcp.CallToolResult, error) {
			if in.TimeoutSec <= 0 {
				in.TimeoutSec = 120
			}
			addr := net.JoinHostPort(in.Host, strconv.Itoa(in.Port))
			start := time.Now()
			get := func() (string, error) { return portState(addr, in.NeedsBanner), nil }
			st, ok, err := waitFor(ctx, time.Duration(in.TimeoutSec)*time.Second, 2*time.Second, get, "open")
			if err != nil {
				return fail(err), nil
			}
			if !ok {
				return errText(fmt.Sprintf("Timed out after %ds: %s is %s.", in.TimeoutSec, addr, st)), nil
			}
			return text(fmt.Sprintf("%s is accepting connections (after %ds).", addr, int(time.Since(start).Seconds()))), nil
		})

	add(s, "vm_remove_nat_port_forward",
		"Remove a NAT port forwarding rule by name (vm_info lists the rules as Forwarding(N)).",
		object([]string{"vm", "name"},
			p("vm", str("VM name or UUID")),
			p("name", str("Rule name given when it was added")),
			p("adapter", def(integer("Network adapter number (1-based)"), 1)),
		),
		func(ctx context.Context, in struct {
			VM      string `json:"vm"`
			Name    string `json:"name"`
			Adapter int    `json:"adapter"`
		}) (*mcp.CallToolResult, error) {
			var err error
			if vmIsActive(ctx, d.vb, in.VM) {
				_, err = d.vb.Run(ctx, "controlvm", in.VM, fmt.Sprintf("natpf%d", in.Adapter), "delete", in.Name)
			} else {
				_, err = d.vb.Run(ctx, "modifyvm", in.VM, fmt.Sprintf("--natpf%d", in.Adapter), "delete", in.Name)
			}
			if err != nil && strings.Contains(err.Error(), "RemoveRedirect") {
				return errText(fmt.Sprintf("NAT adapter %d of %q has no port forward named %q (vm_info lists them as Forwarding(N)).",
					in.Adapter, in.VM, in.Name)), nil
			} else if err != nil {
				return fail(err), nil
			}
			return text(fmt.Sprintf("NAT port forward %q removed from %q.", in.Name, in.VM)), nil
		})

	add(s, "vm_serial_setup",
		"Give a VM a serial console reachable from the host (COM1 as a TCP server on 127.0.0.1:port), so "+
			"vm_serial_exec can type commands and read text output with no screenshots. Ideal for routers "+
			"(MikroTik CHR, VyOS, pfSense) and Linux guests booted with console=ttyS0. The VM must be powered off.",
		object([]string{"vm", "port"},
			p("vm", str("VM name or UUID (powered off)")),
			p("port", integer("Free host TCP port, e.g. 2301")),
		),
		func(ctx context.Context, in struct {
			VM   string `json:"vm"`
			Port int    `json:"port"`
		}) (*mcp.CallToolResult, error) {
			if in.Port < 1 || in.Port > 65535 {
				return errText("port must be 1-65535."), nil
			}
			if st, err := d.vmState(ctx, in.VM); err != nil {
				return fail(err), nil
			} else if st != "poweroff" && st != "aborted" {
				return errText(fmt.Sprintf("VM %q is %s; power it off first (serial ports can only be added while off).", in.VM, st)), nil
			}
			if _, err := d.vb.Run(ctx, "modifyvm", in.VM, "--uart1", "0x3F8", "4",
				"--uart-mode1", "tcpserver", strconv.Itoa(in.Port)); err != nil {
				return fail(err), nil
			}
			return text(fmt.Sprintf("COM1 on %q now listens on 127.0.0.1:%d. Start the VM, then use vm_serial_exec.", in.VM, in.Port)), nil
		})

	add(s, "vm_serial_exec",
		"Type commands on a VM's serial console (set up with vm_serial_setup) and return the text it prints. "+
			"Each command is sent with Enter; output is collected until the console is quiet for idleMs. "+
			"With no commands it just presses Enter and returns what is on screen (e.g. a login prompt).",
		object([]string{"vm"},
			p("vm", str("VM name or UUID (running)")),
			p("commands", array("Lines to type in order (logins too: username, then password). The line ^C sends Ctrl+C instead.", str("A line to type"))),
			p("idleMs", def(integer("Stop reading after this many ms without new output"), 1500)),
			p("timeoutSec", def(integer("Max seconds per command"), 30)),
		),
		func(ctx context.Context, in struct {
			VM         string   `json:"vm"`
			Commands   []string `json:"commands"`
			IdleMs     int      `json:"idleMs"`
			TimeoutSec int      `json:"timeoutSec"`
		}) (*mcp.CallToolResult, error) {
			out, err := d.vb.Run(ctx, "showvminfo", in.VM, "--machinereadable")
			if err != nil {
				return fail(err), nil
			}
			info := vbox.ParseMachineReadable(out)
			port, ok := serialTCPPort(info)
			if !ok {
				return errText(fmt.Sprintf("VM %q has no TCP serial console. Run vm_serial_setup while it is powered off.", in.VM)), nil
			}
			if info["VMState"] != "running" {
				return errText(fmt.Sprintf("VM %q is %s; start it first.", in.VM, info["VMState"])), nil
			}
			idle := time.Duration(max(in.IdleMs, 200)) * time.Millisecond
			limit := time.Duration(max(in.TimeoutSec, 1)) * time.Second
			res, err := serialExec(ctx, net.JoinHostPort("127.0.0.1", port), in.Commands, idle, limit)
			if err != nil {
				return errText(err.Error()), nil
			}
			return text(res), nil
		})
}

// tailLines returns the last n lines of s, keeping only those containing filter.
func tailLines(s, filter string, n int) string {
	if n <= 0 {
		n = 80
	}
	var keep []string
	f := strings.ToLower(filter)
	for _, l := range strings.Split(strings.TrimRight(s, "\n"), "\n") {
		if f == "" || strings.Contains(strings.ToLower(l), f) {
			keep = append(keep, l)
		}
	}
	if len(keep) > n {
		keep = keep[len(keep)-n:]
	}
	if len(keep) == 0 {
		return "(no matching log lines)"
	}
	return strings.Join(keep, "\n")
}

// portState says whether addr has a live service: "open", "closed", "accepting then
// closing", or "connected but silent" (a NAT forward to a guest that is still booting,
// or a client-speaks-first service when needsBanner is set).
func portState(addr string, needsBanner bool) string {
	c, err := net.DialTimeout("tcp", addr, 2*time.Second)
	if err != nil {
		return "closed"
	}
	defer c.Close()
	c.SetReadDeadline(time.Now().Add(1500 * time.Millisecond))
	_, err = c.Read(make([]byte, 1))
	var ne net.Error
	if err == nil {
		return "open"
	}
	if errors.As(err, &ne) && ne.Timeout() {
		if needsBanner {
			return "connected but silent"
		}
		return "open" // waiting for us to speak first (HTTP)
	}
	return "accepting then closing"
}

// serialTCPPort reads uartmode1="tcpserver,2301" from showvminfo.
func serialTCPPort(info map[string]string) (string, bool) {
	mode, port, _ := strings.Cut(info["uartmode1"], ",")
	return port, mode == "tcpserver" && port != ""
}

var serialANSIRe = regexp.MustCompile(`\x1b\[[0-9;?]*[ -/]*[@-~]|\x1b[()][A-Za-z0-9]|\x1b[=>c]|\x1b\][^\x07\x1b]*(?:\x07|\x1b\\)`)

// serialExec types each line on the console and collects what comes back.
func serialExec(ctx context.Context, addr string, lines []string, idle, limit time.Duration) (string, error) {
	var dialer net.Dialer
	dctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	c, err := dialer.DialContext(dctx, "tcp", addr)
	if err != nil {
		return "", fmt.Errorf("cannot reach the serial console at %s (is the VM running? only one client can attach at a time): %w", addr, err)
	}
	defer c.Close()

	if len(lines) == 0 {
		lines = []string{""}
	}
	var out strings.Builder
	for _, l := range lines {
		send := l + "\r"
		if l == "^C" {
			send = "\x03" // Ctrl+C: skip a prompt or stop a running command (ping, tail)
		}
		if _, err := io.WriteString(c, send); err != nil {
			return out.String(), fmt.Errorf("console closed: %w", err)
		}
		out.WriteString(readUntilIdle(ctx, c, idle, limit))
	}
	clean := serialANSIRe.ReplaceAllString(out.String(), "")
	clean = strings.ReplaceAll(strings.ReplaceAll(clean, "\r\n", "\n"), "\r", "\n")
	return strings.TrimSpace(clean), nil
}

// readUntilIdle reads until nothing arrives for idle, limit passes, or ctx ends.
func readUntilIdle(ctx context.Context, c net.Conn, idle, limit time.Duration) string {
	var b strings.Builder
	buf := make([]byte, 4096)
	end := time.Now().Add(limit)
	for time.Now().Before(end) && ctx.Err() == nil {
		c.SetReadDeadline(time.Now().Add(idle))
		n, err := c.Read(buf)
		b.Write(buf[:n])
		if err != nil {
			break // idle timeout or console closed
		}
	}
	return b.String()
}
