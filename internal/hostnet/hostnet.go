// Package hostnet reads the host's ARP/neighbour table so VMs without Guest
// Additions can still be found by MAC address. It runs `arp -a` (Windows, macOS)
// or `ip neigh` (Linux): read-only, no user input in the arguments.
package hostnet

import (
	"context"
	"os/exec"
	"regexp"
	"runtime"
	"strings"
)

// ARPTable returns MAC (12 lowercase hex digits) -> IPv4. Empty on any error.
func ARPTable(ctx context.Context) map[string]string {
	var cmd *exec.Cmd
	if runtime.GOOS == "linux" {
		cmd = exec.CommandContext(ctx, "ip", "neigh")
	} else {
		cmd = exec.CommandContext(ctx, "arp", "-a")
	}
	hideWindow(cmd)
	out, err := cmd.Output()
	if err != nil {
		return map[string]string{}
	}
	return ParseARP(string(out))
}

var (
	ipv4Re = regexp.MustCompile(`\b(\d{1,3}(?:\.\d{1,3}){3})\b`)
	macRe  = regexp.MustCompile(`(?i)\b([0-9a-f]{1,2}(?:[:-][0-9a-f]{1,2}){5})\b`)
	hexRe  = regexp.MustCompile(`[^0-9a-f]`)
)

// ParseARP handles Windows ("  192.168.56.101  08-00-27-8b-3c-9f  dynamic"),
// macOS ("? (192.168.56.101) at 8:0:27:8b:3c:9f on vboxnet0") and Linux
// ("192.168.56.101 dev vboxnet0 lladdr 08:00:27:8b:3c:9f REACHABLE").
func ParseARP(out string) map[string]string {
	res := map[string]string{}
	for _, line := range strings.Split(out, "\n") {
		ip, mac := ipv4Re.FindString(line), macRe.FindString(line)
		if ip == "" || mac == "" {
			continue
		}
		if m := NormMAC(mac); m != "" && m != "ffffffffffff" {
			res[m] = ip
		}
	}
	return res
}

// NormMAC turns 08:00:27:8B:3C:9F, 08-00-27-8b-3c-9f, 8:0:27:8b:3c:9f or
// 0800278B3C9F into 0800278b3c9f. Returns "" if it is not a MAC.
func NormMAC(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	if strings.ContainsAny(s, ":-") {
		parts := strings.FieldsFunc(s, func(r rune) bool { return r == ':' || r == '-' })
		if len(parts) != 6 {
			return ""
		}
		for i, p := range parts {
			if len(p) == 1 {
				parts[i] = "0" + p // macOS drops leading zeros
			}
		}
		s = strings.Join(parts, "")
	}
	if len(s) != 12 || hexRe.MatchString(s) {
		return ""
	}
	return s
}
