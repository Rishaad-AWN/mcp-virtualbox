// Package hostclip sets the host machine's clipboard. It is used by the
// clipboard-paste text-entry path: set the host clipboard, let VirtualBox's
// shared clipboard sync it into the guest, then send a paste chord.
//
// The server runs on the host, so the relevant OS is the host OS (which may be
// Linux, macOS, or Windows regardless of the guest OS). A GUI session and an
// appropriate clipboard helper must be available; headless hosts have no
// clipboard and Set returns a descriptive error.
package hostclip

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"time"
)

// Set places text on the host clipboard, returning a helpful error if no
// clipboard helper is available.
func Set(ctx context.Context, text string) error {
	if runtime.GOOS == "windows" {
		return setWindows(ctx, text)
	}
	return setUnix(ctx, text)
}

type helper struct {
	name string
	args []string
}

func unixHelpers() []helper {
	// Order matters. Prefer the X11 clipboard (xclip/xsel): VirtualBox's shared
	// clipboard reads the X11 selection (via XWayland on Wayland sessions), and a
	// Wayland-only wl-copy does NOT propagate to X11 — so a wl-copy-set clipboard
	// never reaches the guest. pbcopy covers macOS. wl-copy is a last resort for
	// pure-Wayland hosts without XWayland (where VBox clipboard sharing generally
	// won't work anyway).
	return []helper{
		{"xclip", []string{"-selection", "clipboard"}},
		{"xsel", []string{"--clipboard", "--input"}},
		{"pbcopy", nil}, // macOS
		{"wl-copy", nil},
	}
}

func setUnix(ctx context.Context, text string) error {
	var tried []string
	for _, h := range unixHelpers() {
		if _, err := exec.LookPath(h.name); err != nil {
			continue
		}
		tried = append(tried, h.name)
		if err := runClipboardHelper(ctx, h, text); err != nil {
			return err
		}
		return nil
	}
	if len(tried) == 0 {
		return fmt.Errorf("no host clipboard helper found; install one of: wl-copy (wl-clipboard), xclip, xsel, or pbcopy")
	}
	return fmt.Errorf("clipboard helpers %v were present but none succeeded", tried)
}

// runClipboardHelper feeds text to a clipboard helper's stdin. wl-copy, xclip,
// and xsel fork a daemon that STAYS ALIVE to serve the selection, so we must
// not wait for the process to exit or capture its output through a pipe (the
// daemon inherits the pipe's write end and Wait would block until the clipboard
// is next replaced). We give the helper a short window: if it exits quickly
// with an error (e.g. no display), report it; if it is still running after
// consuming stdin, it has daemonized successfully.
func runClipboardHelper(ctx context.Context, h helper, text string) error {
	// stderr goes to a real file (not an os.Pipe) so a surviving daemon child
	// can't keep us blocked.
	errFile, err := os.CreateTemp("", "mcpvb-cliperr-*")
	if err != nil {
		return fmt.Errorf("creating temp file: %w", err)
	}
	defer os.Remove(errFile.Name())
	defer errFile.Close()

	cmd := exec.Command(h.name, h.args...) // not CommandContext: ctx cancel would kill the serving daemon
	cmd.Stdin = strings.NewReader(text)    // UTF-8; all these helpers accept it
	cmd.Stdout = nil                       // /dev/null
	cmd.Stderr = errFile
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("%s: %w", h.name, err)
	}

	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }() // reaps whenever the daemon eventually exits

	select {
	case werr := <-done:
		if werr != nil {
			msg, _ := os.ReadFile(errFile.Name())
			detail := strings.TrimSpace(string(msg))
			if detail == "" {
				detail = werr.Error()
			}
			return fmt.Errorf("%s failed to set clipboard: %s (is a graphical session available?)", h.name, detail)
		}
		return nil // exited cleanly (e.g. pbcopy, which does not daemonize)
	case <-time.After(750 * time.Millisecond):
		// Still running after consuming stdin: it daemonized to serve the
		// selection. That is success; leave it running.
		return nil
	}
}

// setWindows writes the text as a UTF-8 file and hands it to Set-Clipboard so
// Unicode survives (piping to clip.exe mangles non-ASCII via the console code
// page).
func setWindows(ctx context.Context, text string) error {
	f, err := os.CreateTemp("", "mcpvb-clip-*.txt")
	if err != nil {
		return fmt.Errorf("creating temp clipboard file: %w", err)
	}
	path := f.Name()
	defer os.Remove(path)
	if _, err := f.WriteString(text); err != nil {
		f.Close()
		return err
	}
	f.Close()

	ps := "Set-Clipboard -Value (Get-Content -LiteralPath '" + path + "' -Raw -Encoding UTF8)"
	cmd := exec.CommandContext(ctx, "powershell", "-NoProfile", "-NonInteractive", "-Command", ps)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			msg = err.Error()
		}
		return fmt.Errorf("powershell Set-Clipboard failed: %s", msg)
	}
	return nil
}
