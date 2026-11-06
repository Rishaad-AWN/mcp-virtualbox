package tools

import (
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/knownhosts"
)

var knownHostsMu sync.Mutex

// knownHostsPath is where vm_exec_ssh remembers guest host keys (its own file, so the
// user's ~/.ssh/known_hosts is never touched).
func knownHostsPath() string {
	if p := os.Getenv("VBOX_MCP_KNOWN_HOSTS"); p != "" {
		return p
	}
	dir, err := os.UserConfigDir()
	if err != nil {
		dir = os.TempDir()
	}
	return filepath.Join(dir, "mcp-virtualbox", "known_hosts")
}

// tofuHostKey trusts a host's key the first time it is seen and refuses a different
// key later (a man-in-the-middle, or a reinstalled VM). replace=true forgets the old
// key and trusts the new one.
func tofuHostKey(path string, replace bool) ssh.HostKeyCallback {
	return func(hostname string, remote net.Addr, key ssh.PublicKey) error {
		knownHostsMu.Lock()
		defer knownHostsMu.Unlock()
		if cb, err := knownhosts.New(path); err == nil {
			err = cb(hostname, remote, key)
			var ke *knownhosts.KeyError
			switch {
			case err == nil:
				return nil
			case !errors.As(err, &ke):
				return err
			case len(ke.Want) > 0 && !replace:
				return fmt.Errorf("the SSH host key for %s has CHANGED since it was first trusted. Either the VM "+
					"was reinstalled/recreated (or another VM now uses this address/port), or someone is "+
					"intercepting the connection. If you know the VM was replaced, call again with "+
					"acceptNewHostKey=true", hostname)
			}
		} else if !os.IsNotExist(err) {
			return fmt.Errorf("reading %s: %w", path, err)
		}
		return rememberHostKey(path, hostname, key)
	}
}

// rememberHostKey drops any old line for hostname and appends the new key.
func rememberHostKey(path, hostname string, key ssh.PublicKey) error {
	host := knownhosts.Normalize(hostname)
	var keep []string
	if data, err := os.ReadFile(path); err == nil {
		for _, line := range strings.Split(strings.TrimSpace(string(data)), "\n") {
			if f := strings.Fields(line); len(f) > 0 && !hasHost(f[0], host) {
				keep = append(keep, line)
			}
		}
	}
	keep = append(keep, knownhosts.Line([]string{host}, key))
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	return os.WriteFile(path, []byte(strings.Join(keep, "\n")+"\n"), 0o600)
}

func hasHost(hosts, host string) bool {
	for _, h := range strings.Split(hosts, ",") {
		if h == host {
			return true
		}
	}
	return false
}
