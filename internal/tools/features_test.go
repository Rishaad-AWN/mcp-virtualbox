package tools

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"net"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"golang.org/x/crypto/ssh"
)

func newKey(t *testing.T) ssh.PublicKey {
	pub, _, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	k, err := ssh.NewPublicKey(pub)
	if err != nil {
		t.Fatal(err)
	}
	return k
}

func TestTOFUHostKey(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sub", "known_hosts")
	addr := &net.TCPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 2222}
	host := "127.0.0.1:2222"
	k1, k2 := newKey(t), newKey(t)

	if err := tofuHostKey(path, false)(host, addr, k1); err != nil {
		t.Fatalf("first contact should be trusted: %v", err)
	}
	if err := tofuHostKey(path, false)(host, addr, k1); err != nil {
		t.Fatalf("same key should pass: %v", err)
	}
	err := tofuHostKey(path, false)(host, addr, k2)
	if err == nil || !strings.Contains(err.Error(), "CHANGED") {
		t.Fatalf("changed key must be refused, got %v", err)
	}
	if err := tofuHostKey(path, true)(host, addr, k2); err != nil {
		t.Fatalf("acceptNewHostKey should replace: %v", err)
	}
	if err := tofuHostKey(path, false)(host, addr, k2); err != nil {
		t.Fatalf("new key should now be trusted: %v", err)
	}
	if err := tofuHostKey(path, false)(host, addr, k1); err == nil {
		t.Fatal("old key must no longer be trusted")
	}
	// a different port is a different host
	other := &net.TCPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 2223}
	if err := tofuHostKey(path, false)("127.0.0.1:2223", other, k1); err != nil {
		t.Fatalf("other port first contact: %v", err)
	}
}

func TestTailLines(t *testing.T) {
	log := "a\nERROR one\nb\nerror two\nc\n"
	if got := tailLines(log, "", 2); got != "error two\nc" {
		t.Fatalf("tail: %q", got)
	}
	if got := tailLines(log, "Error", 10); got != "ERROR one\nerror two" {
		t.Fatalf("filter: %q", got)
	}
	if got := tailLines(log, "zzz", 10); !strings.Contains(got, "no matching") {
		t.Fatalf("none: %q", got)
	}
}

func listen(t *testing.T, serve func(net.Conn)) string {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { ln.Close() })
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			go serve(c)
		}
	}()
	return ln.Addr().String()
}

func TestPortState(t *testing.T) {
	banner := listen(t, func(c net.Conn) { c.Write([]byte("SSH-2.0-test\r\n")); time.Sleep(time.Second); c.Close() })
	silent := listen(t, func(c net.Conn) { time.Sleep(3 * time.Second); c.Close() })
	natDown := listen(t, func(c net.Conn) { c.Close() })
	if st := portState(banner, true); st != "open" {
		t.Errorf("banner: %s", st)
	}
	if st := portState(silent, true); st != "connected but silent" {
		t.Errorf("silent with needsBanner (NAT forward, guest booting): %s", st)
	}
	if st := portState(silent, false); st != "open" {
		t.Errorf("silent without needsBanner (HTTP-like): %s", st)
	}
	if st := portState(natDown, true); st != "accepting then closing" {
		t.Errorf("nat down: %s", st)
	}
	ln, _ := net.Listen("tcp", "127.0.0.1:0")
	dead := ln.Addr().String()
	ln.Close()
	if st := portState(dead, true); st != "closed" {
		t.Errorf("closed: %s", st)
	}
}

func TestSerialTCPPort(t *testing.T) {
	if p, ok := serialTCPPort(map[string]string{"uartmode1": "tcpserver,2301"}); !ok || p != "2301" {
		t.Fatalf("got %q %v", p, ok)
	}
	if _, ok := serialTCPPort(map[string]string{"uartmode1": "disconnected"}); ok {
		t.Fatal("disconnected is not a TCP console")
	}
	if _, ok := serialTCPPort(map[string]string{}); ok {
		t.Fatal("no uart")
	}
}

// fake RouterOS-ish console: login, then echoes "ran: <cmd>" and a prompt, with ANSI colour.
func TestSerialExec(t *testing.T) {
	addr := listen(t, func(c net.Conn) {
		defer c.Close()
		buf := make([]byte, 256)
		var line strings.Builder
		c.Write([]byte("\x1b[2J\r\nMikroTik Login: "))
		step := 0
		for {
			n, err := c.Read(buf)
			if err != nil {
				return
			}
			for _, b := range buf[:n] {
				if b == 3 {
					c.Write([]byte("^C\r\n[admin@R1] > "))
					continue
				}
				if b != '\r' {
					line.WriteByte(b)
					continue
				}
				cmd := line.String()
				line.Reset()
				switch step {
				case 0:
					c.Write([]byte("\r\nPassword: "))
				case 1:
					c.Write([]byte("\r\n\x1b[01;32m[admin@R1] >\x1b[0m "))
				default:
					c.Write([]byte("\r\nran: " + cmd + "\r\n[admin@R1] > "))
				}
				step++
			}
		}
	})
	out, err := serialExec(context.Background(), addr, []string{"admin", "pw", "/ip address print", "^C"}, 300*time.Millisecond, 5*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(out, "\x1b") || !strings.Contains(out, "ran: /ip address print") || !strings.Contains(out, "[admin@R1] >") ||
		!strings.Contains(out, "^C") {
		t.Fatalf("got %q", out)
	}

	if _, err := serialExec(context.Background(), "127.0.0.1:1", nil, 100*time.Millisecond, time.Second); err == nil ||
		!strings.Contains(err.Error(), "cannot reach the serial console") {
		t.Fatalf("unreachable console should explain, got %v", err)
	}
}
