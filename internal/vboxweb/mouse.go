// Package vboxweb performs absolute mouse clicks against a running VM by
// talking SOAP to the VirtualBox web service (vboxwebsrv), keeping the server a
// single Go binary with no external runtime.
//
// VBoxManage has no CLI command for mouse events, so the Main API is the only
// way to drive the guest pointer. Reaching it from Go without cgo/XPCOM means
// using the web service. The service is started automatically on demand the
// first time a mouse click is needed (see ensureStarted); no manual setup is
// required in the default configuration.
//
// Configuration via environment:
//
//	VBOXWEB_ENDPOINT    service URL (default http://127.0.0.1:18083/)
//	VBOXWEB_USER        logon username (default empty -> anonymous/null auth)
//	VBOXWEB_PASSWORD    logon password (default empty)
//	VBOXWEB_AUTOSTART   "false" to disable auto-starting vboxwebsrv
//	VBOXWEB_BIN         path to the vboxwebsrv binary (else derived/PATH)
package vboxweb

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"sync"
	"time"
)

// MouseButton bit flags, matching VirtualBox's MouseButtonState.
const (
	buttonLeft   = 0x01
	buttonRight  = 0x02
	buttonMiddle = 0x04
)

// Client talks to a vboxwebsrv SOAP endpoint and can start the service on
// demand.
type Client struct {
	endpoint string
	user     string
	password string
	http     *http.Client

	// autostart configuration.
	autostart      bool
	vboxwebsrvBin  string // path/name of vboxwebsrv
	vboxManagePath string // used to set null auth before first start
	host           string // parsed from endpoint, for spawning + reachability
	port           string

	mu      sync.Mutex // serialises ensureStarted
	started bool       // we (or a prior check) confirmed the service is up
}

// NewFromEnv builds a Client from the VBOXWEB_* environment variables.
// vboxManagePath is used to locate vboxwebsrv (installed alongside it) and to
// set the web service to anonymous auth on first auto-start.
func NewFromEnv(timeout time.Duration, vboxManagePath string) *Client {
	endpoint := os.Getenv("VBOXWEB_ENDPOINT")
	if endpoint == "" {
		// Use 127.0.0.1 (not "localhost"): vboxwebsrv binds IPv4, while
		// "localhost" often resolves to ::1 first and the dial is refused.
		endpoint = "http://127.0.0.1:18083/"
	}
	host, port := parseHostPort(endpoint)

	return &Client{
		endpoint:       endpoint,
		user:           os.Getenv("VBOXWEB_USER"),
		password:       os.Getenv("VBOXWEB_PASSWORD"),
		http:           &http.Client{Timeout: timeout},
		autostart:      os.Getenv("VBOXWEB_AUTOSTART") != "false",
		vboxwebsrvBin:  resolveVboxwebsrv(vboxManagePath),
		vboxManagePath: vboxManagePath,
		host:           host,
		port:           port,
	}
}

func parseHostPort(endpoint string) (host, port string) {
	host, port = "127.0.0.1", "18083"
	if u, err := url.Parse(endpoint); err == nil {
		if h := u.Hostname(); h != "" {
			host = h
		}
		if p := u.Port(); p != "" {
			port = p
		}
	}
	return host, port
}

// resolveVboxwebsrv locates the vboxwebsrv binary: VBOXWEB_BIN override, then
// alongside VBoxManage, then bare name for PATH lookup.
func resolveVboxwebsrv(vboxManagePath string) string {
	if b := os.Getenv("VBOXWEB_BIN"); b != "" {
		return b
	}
	name := "vboxwebsrv"
	if runtime.GOOS == "windows" {
		name = "vboxwebsrv.exe"
	}
	if vboxManagePath != "" {
		cand := filepath.Join(filepath.Dir(vboxManagePath), name)
		if fi, err := os.Stat(cand); err == nil && !fi.IsDir() {
			return cand
		}
	}
	return name
}

// Click moves the absolute pointer to (x, y) and presses/releases button. When
// double is true it issues a second press/release. The VM must have USB tablet
// mouse mode enabled (vm_create does this) for absolute positioning to work.
//
// If the web service is unreachable and autostart is enabled, Click starts
// vboxwebsrv on demand and retries once.
func (c *Client) Click(ctx context.Context, vm string, x, y int, button string, double bool) error {
	flag := buttonFlag(button)
	return c.do(ctx, vm, func(send sendFunc) error {
		seq := []int{flag, 0}
		if double {
			seq = append(seq, flag, 0)
		}
		for _, b := range seq {
			if err := send(x, y, 0, 0, b); err != nil {
				return err
			}
			if b == flag {
				time.Sleep(50 * time.Millisecond)
			}
		}
		return nil
	})
}

// Move positions the absolute pointer at (x, y) without pressing any button.
func (c *Client) Move(ctx context.Context, vm string, x, y int) error {
	return c.do(ctx, vm, func(send sendFunc) error {
		return send(x, y, 0, 0, 0)
	})
}

// Scroll positions the pointer at (x, y) and turns the wheel: dz is the vertical
// wheel delta (positive = up, negative = down), dw the horizontal delta.
func (c *Client) Scroll(ctx context.Context, vm string, x, y, dz, dw int) error {
	return c.do(ctx, vm, func(send sendFunc) error {
		if err := send(x, y, 0, 0, 0); err != nil {
			return err
		}
		return send(x, y, dz, dw, 0)
	})
}

// Drag presses button at (x1, y1), moves to (x2, y2) in steps while held, then
// releases. Intermediate moves make the drag register with the guest.
func (c *Client) Drag(ctx context.Context, vm string, x1, y1, x2, y2 int, button string) error {
	flag := buttonFlag(button)
	return c.do(ctx, vm, func(send sendFunc) error {
		if err := send(x1, y1, 0, 0, flag); err != nil {
			return err
		}
		time.Sleep(50 * time.Millisecond)
		const steps = 8
		for i := 1; i <= steps; i++ {
			xi := x1 + (x2-x1)*i/steps
			yi := y1 + (y2-y1)*i/steps
			if err := send(xi, yi, 0, 0, flag); err != nil {
				return err
			}
			time.Sleep(15 * time.Millisecond)
		}
		time.Sleep(50 * time.Millisecond)
		return send(x2, y2, 0, 0, 0) // release
	})
}

func buttonFlag(button string) int {
	switch button {
	case "right":
		return buttonRight
	case "middle":
		return buttonMiddle
	default:
		return buttonLeft
	}
}

// sendFunc sends one absolute mouse event to a locked VM's IMouse.
type sendFunc func(x, y, dz, dw, buttons int) error

// do runs a mouse action, and if the web service is unreachable and autostart
// is enabled, brings vboxwebsrv up and retries once.
func (c *Client) do(ctx context.Context, vm string, action func(send sendFunc) error) error {
	err := c.withMouse(ctx, vm, action)
	if err == nil || !isTransport(err) {
		return err
	}
	if !c.autostart {
		return c.connErr(err)
	}
	if startErr := c.ensureStarted(ctx); startErr != nil {
		return fmt.Errorf("%v\n(auto-start failed: %v)", c.connErr(err), startErr)
	}
	if retryErr := c.withMouse(ctx, vm, action); retryErr != nil {
		if isTransport(retryErr) {
			return c.connErr(retryErr)
		}
		return retryErr
	}
	return nil
}

// withMouse establishes a shared lock on the running VM, resolves its IMouse,
// and invokes action with a sender for absolute mouse events. It always
// unlocks and logs off, even on error.
func (c *Client) withMouse(ctx context.Context, vm string, action func(send sendFunc) error) error {
	vbox, err := c.call(ctx, "IWebsessionManager_logon",
		elem("username", c.user), elem("password", c.password))
	if err != nil {
		return err
	}
	defer c.call(ctx, "IWebsessionManager_logoff", elem("refIVirtualBox", vbox)) //nolint:errcheck

	machine, err := c.call(ctx, "IVirtualBox_findMachine", elem("_this", vbox), elem("nameOrId", vm))
	if err != nil {
		return fmt.Errorf("findMachine %q: %w", vm, err)
	}
	session, err := c.call(ctx, "IWebsessionManager_getSessionObject", elem("refIVirtualBox", vbox))
	if err != nil {
		return err
	}
	if _, err := c.call(ctx, "IMachine_lockMachine",
		elem("_this", machine), elem("session", session), elem("lockType", "Shared")); err != nil {
		return fmt.Errorf("lockMachine (is the VM running?): %w", err)
	}
	defer c.call(ctx, "ISession_unlockMachine", elem("_this", session)) //nolint:errcheck

	console, err := c.call(ctx, "ISession_getConsole", elem("_this", session))
	if err != nil {
		return err
	}
	mouse, err := c.call(ctx, "IConsole_getMouse", elem("_this", console))
	if err != nil {
		return err
	}

	send := func(x, y, dz, dw, buttons int) error {
		if _, err := c.call(ctx, "IMouse_putMouseEventAbsolute",
			elem("_this", mouse), elem("x", x), elem("y", y),
			elem("dz", dz), elem("dw", dw), elem("buttonState", buttons)); err != nil {
			return fmt.Errorf("putMouseEventAbsolute: %w", err)
		}
		return nil
	}
	return action(send)
}

// connErr wraps a transport-level failure with actionable setup guidance,
// used when auto-start is disabled or could not bring the service up.
func (c *Client) connErr(err error) error {
	return fmt.Errorf("cannot reach VirtualBox web service at %s (%v).\n"+
		"Mouse control needs vboxwebsrv running. Start it with:\n"+
		"  VBoxManage setproperty websrvauthlibrary null\n"+
		"  vboxwebsrv --host %s --port %s &\n"+
		"(or set VBOXWEB_AUTOSTART=true and ensure %q is on PATH)",
		c.endpoint, err, c.host, c.port, c.vboxwebsrvBin)
}

// transportError marks an HTTP transport failure (service down / refused), as
// opposed to a SOAP fault returned by a running service.
type transportError struct{ err error }

func (e *transportError) Error() string { return e.err.Error() }
func (e *transportError) Unwrap() error { return e.err }

func isTransport(err error) bool {
	var te *transportError
	return errors.As(err, &te)
}

// ensureStarted brings vboxwebsrv up if it isn't already reachable. It is safe
// to call concurrently; only one start is attempted at a time.
func (c *Client) ensureStarted(ctx context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.reachable(300 * time.Millisecond) {
		c.started = true
		return nil
	}

	// Anonymous logon requires the "null" auth library. Only set it when the
	// user hasn't supplied credentials (otherwise respect their auth setup).
	if c.user == "" && c.vboxManagePath != "" {
		cmd := exec.CommandContext(ctx, c.vboxManagePath, "setproperty", "websrvauthlibrary", "null")
		if out, err := cmd.CombinedOutput(); err != nil {
			return fmt.Errorf("setting websrvauthlibrary=null: %v: %s", err, strings.TrimSpace(string(out)))
		}
	}

	logPath := filepath.Join(os.TempDir(), "vboxwebsrv.log")
	logFile, _ := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)

	// #nosec G204 -- binary path/host/port come from env/config, not tool input.
	cmd := exec.Command(c.vboxwebsrvBin, "--host", c.host, "--port", c.port, "--timeout", "0")
	if logFile != nil {
		cmd.Stdout = logFile
		cmd.Stderr = logFile
	}
	detach(cmd) // run in its own process group so it outlives us

	if err := cmd.Start(); err != nil {
		if logFile != nil {
			logFile.Close()
		}
		return fmt.Errorf("starting %s: %v", c.vboxwebsrvBin, err)
	}
	// We intentionally do not Wait: vboxwebsrv is a long-lived daemon. Release
	// our handle so it is reparented rather than becoming a zombie.
	go func() { _ = cmd.Process.Release() }()

	// Wait for the service to accept connections.
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		if c.reachable(300 * time.Millisecond) {
			c.started = true
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(250 * time.Millisecond):
		}
	}
	return fmt.Errorf("%s started but %s:%s did not become reachable within 10s (log: %s)",
		c.vboxwebsrvBin, c.host, c.port, logPath)
}

// reachable reports whether something is listening on the endpoint's host:port.
func (c *Client) reachable(timeout time.Duration) bool {
	conn, err := net.DialTimeout("tcp", net.JoinHostPort(c.host, c.port), timeout)
	if err != nil {
		return false
	}
	_ = conn.Close()
	return true
}

type xmlElem struct {
	name string
	val  string
}

func elem(name string, val any) xmlElem {
	return xmlElem{name: name, val: fmt.Sprintf("%v", val)}
}

var returnvalRe = regexp.MustCompile(`<(?:\w+:)?returnval[^>]*>([^<]*)</(?:\w+:)?returnval>`)
var faultRe = regexp.MustCompile(`(?s)<(?:\w+:)?faultstring[^>]*>(.*?)</(?:\w+:)?faultstring>`)

// call issues one SOAP operation and returns the <returnval> text (empty for
// void methods).
func (c *Client) call(ctx context.Context, op string, params ...xmlElem) (string, error) {
	var body strings.Builder
	body.WriteString(`<?xml version="1.0" encoding="UTF-8"?>`)
	body.WriteString(`<SOAP-ENV:Envelope xmlns:SOAP-ENV="http://schemas.xmlsoap.org/soap/envelope/"` +
		` xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance"` +
		` xmlns:xsd="http://www.w3.org/2001/XMLSchema"` +
		` xmlns:vbox="http://www.virtualbox.org/">`)
	body.WriteString("<SOAP-ENV:Body>")
	fmt.Fprintf(&body, "<vbox:%s>", op)
	for _, p := range params {
		fmt.Fprintf(&body, "<%s>%s</%s>", p.name, escapeXML(p.val), p.name)
	}
	fmt.Fprintf(&body, "</vbox:%s>", op)
	body.WriteString("</SOAP-ENV:Body></SOAP-ENV:Envelope>")

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.endpoint, strings.NewReader(body.String()))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "text/xml; charset=utf-8")
	req.Header.Set("SOAPAction", "")

	resp, err := c.http.Do(req)
	if err != nil {
		return "", &transportError{err: err}
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		return "", err
	}
	text := string(raw)

	if m := faultRe.FindStringSubmatch(text); m != nil {
		return "", fmt.Errorf("SOAP fault: %s", strings.TrimSpace(m[1]))
	}
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("HTTP %d from web service", resp.StatusCode)
	}
	if m := returnvalRe.FindStringSubmatch(text); m != nil {
		return m[1], nil
	}
	return "", nil // void method, no returnval
}

func escapeXML(s string) string {
	var b bytes.Buffer
	_ = xmlEscape(&b, s)
	return b.String()
}

// xmlEscape is a tiny dependency-free replacement for xml.EscapeText that keeps
// output stable across Go versions.
func xmlEscape(w io.Writer, s string) error {
	repl := strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;", `"`, "&quot;", "'", "&apos;")
	_, err := io.WriteString(w, repl.Replace(s))
	return err
}
