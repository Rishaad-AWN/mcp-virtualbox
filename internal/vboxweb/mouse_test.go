package vboxweb

import (
	"context"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestEscapeXML(t *testing.T) {
	if got := escapeXML(`a<b>&"c"`); got != "a&lt;b&gt;&amp;&quot;c&quot;" {
		t.Errorf("escapeXML = %q", got)
	}
}

func TestReturnvalRegex(t *testing.T) {
	body := `<foo:IWebsessionManager_logonResponse><returnval>vbox-123</returnval></foo:IWebsessionManager_logonResponse>`
	m := returnvalRe.FindStringSubmatch(body)
	if m == nil || m[1] != "vbox-123" {
		t.Errorf("returnval match = %v", m)
	}
}

// TestClickFlow drives a full Click against a fake vboxwebsrv, asserting the
// SOAP operation sequence and that coordinates/buttons are sent.
func TestClickFlow(t *testing.T) {
	var ops []string
	var sawCoords, sawPressRelease bool

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		s := string(body)
		var op, ret string
		switch {
		case strings.Contains(s, "IWebsessionManager_logon>"):
			op, ret = "logon", "vbox-ref"
		case strings.Contains(s, "IVirtualBox_findMachine"):
			op, ret = "findMachine", "machine-ref"
		case strings.Contains(s, "IWebsessionManager_getSessionObject"):
			op, ret = "getSessionObject", "session-ref"
		case strings.Contains(s, "IMachine_lockMachine"):
			op = "lockMachine"
		case strings.Contains(s, "ISession_getConsole"):
			op, ret = "getConsole", "console-ref"
		case strings.Contains(s, "IConsole_getMouse"):
			op, ret = "getMouse", "mouse-ref"
		case strings.Contains(s, "IMouse_putMouseEventAbsolute"):
			op = "putMouseEventAbsolute"
			if strings.Contains(s, "<x>100</x>") && strings.Contains(s, "<y>200</y>") {
				sawCoords = true
			}
			if strings.Contains(s, "<buttonState>1</buttonState>") {
				sawPressRelease = true
			}
		case strings.Contains(s, "ISession_unlockMachine"):
			op = "unlockMachine"
		case strings.Contains(s, "IWebsessionManager_logoff"):
			op = "logoff"
		}
		if op != "" {
			ops = append(ops, op)
		}
		w.Header().Set("Content-Type", "text/xml")
		io.WriteString(w, `<?xml version="1.0"?><env><returnval>`+ret+`</returnval></env>`)
	}))
	defer srv.Close()

	c := &Client{endpoint: srv.URL, http: &http.Client{Timeout: 5 * time.Second}}
	if err := c.Click(context.Background(), "MyVM", 100, 200, "left", false); err != nil {
		t.Fatalf("Click: %v", err)
	}

	want := []string{"logon", "findMachine", "getSessionObject", "lockMachine", "getConsole", "getMouse", "putMouseEventAbsolute", "putMouseEventAbsolute"}
	for _, w := range want {
		if !containsStr(ops, w) {
			t.Errorf("missing op %q in %v", w, ops)
		}
	}
	if !sawCoords {
		t.Error("coordinates 100,200 not sent")
	}
	if !sawPressRelease {
		t.Error("button press (buttonState=1) not sent")
	}
}

func TestMoveSendsSingleNoButtonEvent(t *testing.T) {
	var events int
	var sawZeroButton bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		s := string(body)
		ret := ""
		switch {
		case strings.Contains(s, "IWebsessionManager_logon>"):
			ret = "vbox"
		case strings.Contains(s, "IVirtualBox_findMachine"):
			ret = "machine"
		case strings.Contains(s, "IWebsessionManager_getSessionObject"):
			ret = "session"
		case strings.Contains(s, "ISession_getConsole"):
			ret = "console"
		case strings.Contains(s, "IConsole_getMouse"):
			ret = "mouse"
		case strings.Contains(s, "IMouse_putMouseEventAbsolute"):
			events++
			if strings.Contains(s, "<buttonState>0</buttonState>") && strings.Contains(s, "<x>10</x>") {
				sawZeroButton = true
			}
		}
		w.Header().Set("Content-Type", "text/xml")
		io.WriteString(w, `<e><returnval>`+ret+`</returnval></e>`)
	}))
	defer srv.Close()

	c := &Client{endpoint: srv.URL, http: &http.Client{Timeout: 5 * time.Second}}
	if err := c.Move(context.Background(), "VM", 10, 20); err != nil {
		t.Fatal(err)
	}
	if events != 1 {
		t.Errorf("Move sent %d mouse events, want 1", events)
	}
	if !sawZeroButton {
		t.Error("Move should send a no-button event at the coordinates")
	}
}

func TestDragSendsPressMovesRelease(t *testing.T) {
	var press, release, total int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		s := string(body)
		ret := "x"
		if strings.Contains(s, "IMouse_putMouseEventAbsolute") {
			total++
			if strings.Contains(s, "<buttonState>1</buttonState>") {
				press++
			}
			if strings.Contains(s, "<buttonState>0</buttonState>") {
				release++
			}
		}
		w.Header().Set("Content-Type", "text/xml")
		io.WriteString(w, `<e><returnval>`+ret+`</returnval></e>`)
	}))
	defer srv.Close()

	c := &Client{endpoint: srv.URL, http: &http.Client{Timeout: 5 * time.Second}}
	if err := c.Drag(context.Background(), "VM", 0, 0, 100, 100, "left"); err != nil {
		t.Fatal(err)
	}
	if press < 1 {
		t.Errorf("Drag sent %d press events, want >=1", press)
	}
	if release != 1 {
		t.Errorf("Drag sent %d release events, want exactly 1", release)
	}
	if total < 3 {
		t.Errorf("Drag sent %d events total, want >=3 (press + moves + release)", total)
	}
}

func TestClickConnRefused(t *testing.T) {
	// Nothing listening on this port -> actionable setup error.
	c := &Client{endpoint: "http://127.0.0.1:1/", http: &http.Client{Timeout: time.Second}}
	err := c.Click(context.Background(), "MyVM", 1, 1, "left", false)
	if err == nil || !strings.Contains(err.Error(), "vboxwebsrv") {
		t.Errorf("expected setup guidance, got %v", err)
	}
}

func TestEnsureStartedAlreadyReachable(t *testing.T) {
	// A listener already bound to the port -> ensureStarted must NOT spawn
	// anything and should succeed immediately.
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	_, port, _ := net.SplitHostPort(ln.Addr().String())

	c := &Client{
		host:          "127.0.0.1",
		port:          port,
		autostart:     true,
		vboxwebsrvBin: "/nonexistent/should-not-be-run", // proves no spawn happens
		http:          &http.Client{Timeout: time.Second},
	}
	if err := c.ensureStarted(context.Background()); err != nil {
		t.Fatalf("ensureStarted: %v", err)
	}
	if !c.started {
		t.Error("expected started=true")
	}
}

func TestEnsureStartedSpawnFails(t *testing.T) {
	// Grab a free port and immediately close it, so it is unreachable.
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	_, port, _ := net.SplitHostPort(ln.Addr().String())
	ln.Close()

	c := &Client{
		host:          "127.0.0.1",
		port:          port,
		autostart:     true,
		vboxwebsrvBin: "/nonexistent/vboxwebsrv-xyz",
		// vboxManagePath empty -> skip setproperty; go straight to spawn.
		http: &http.Client{Timeout: 200 * time.Millisecond},
	}
	if err := c.ensureStarted(context.Background()); err == nil {
		t.Fatal("expected spawn failure, got nil")
	}
}

func TestClickAutostartDisabledGivesGuidance(t *testing.T) {
	c := &Client{
		endpoint:      "http://127.0.0.1:1/",
		host:          "127.0.0.1",
		port:          "1",
		vboxwebsrvBin: "vboxwebsrv",
		autostart:     false,
		http:          &http.Client{Timeout: time.Second},
	}
	err := c.Click(context.Background(), "MyVM", 1, 1, "left", false)
	if err == nil || !strings.Contains(err.Error(), "vboxwebsrv") {
		t.Errorf("expected setup guidance, got %v", err)
	}
}

func containsStr(xs []string, s string) bool {
	for _, x := range xs {
		if x == s {
			return true
		}
	}
	return false
}
