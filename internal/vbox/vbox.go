// Package vbox wraps the VBoxManage CLI. Every VirtualBox interaction in the
// server flows through Client.Run so that error formatting, dry-run handling,
// debug logging and CRLF normalisation happen in exactly one place.
package vbox

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os/exec"
	"regexp"
	"strings"
)

// Client executes VBoxManage commands using a shared Config.
type Client struct {
	cfg *Config
}

// New returns a Client bound to cfg.
func New(cfg *Config) *Client { return &Client{cfg: cfg} }

// Config exposes the underlying configuration (path, screenshot dir, etc.).
func (c *Client) Config() *Config { return c.cfg }

// Run invokes VBoxManage with args and returns normalised stdout. It never uses
// a shell, so arguments are passed verbatim with no injection risk. On failure
// it returns a *VBoxError carrying the parsed VBOX_E_* code.
func (c *Client) Run(ctx context.Context, args ...string) (string, error) {
	command := "VBoxManage " + strings.Join(args, " ")

	if c.cfg.DryRun {
		c.cfg.Logf("[dry-run] %s", command)
		return "", nil
	}

	c.cfg.Logf("[exec] %s", command)

	cmd := exec.CommandContext(ctx, c.cfg.VBoxManagePath, args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		sub := "unknown"
		if len(args) > 0 {
			sub = args[0]
		}
		detail := strings.TrimSpace(stderr.String())
		if detail == "" {
			detail = err.Error()
		}
		msg := fmt.Sprintf("VBoxManage %s failed: %s", sub, detail)
		c.cfg.Logf("[error] %s", msg)
		return "", newVBoxError(sub, stderr.String(), msg)
	}

	out := strings.ReplaceAll(stdout.String(), "\r\n", "\n")
	if len(args) > 0 {
		c.cfg.Logf("[ok] %s (%d bytes)", args[0], len(out))
	}
	return out, nil
}

// VBoxError represents a failed VBoxManage invocation.
type VBoxError struct {
	// Code is the parsed VBOX_E_* code, or "UNKNOWN".
	Code string
	// Command is the VBoxManage subcommand that failed (e.g. "startvm").
	Command string
	// Message is the human-readable error, including VBoxManage stderr.
	Message string
}

func (e *VBoxError) Error() string { return e.Message }

var vboxErrCodeRe = regexp.MustCompile(`code (VBOX_E_\w+)`)

func newVBoxError(command, stderr, message string) *VBoxError {
	code := "UNKNOWN"
	if m := vboxErrCodeRe.FindStringSubmatch(stderr); m != nil {
		code = m[1]
	}
	return &VBoxError{Code: code, Command: command, Message: message}
}

// FormatError renders any error for return to the MCP caller. VBoxErrors are
// prefixed with their code; other errors return their message. Raw VBoxManage
// stderr never leaks unformatted.
func FormatError(err error) string {
	if err == nil {
		return ""
	}
	var ve *VBoxError
	if errors.As(err, &ve) {
		return fmt.Sprintf("[%s] %s", ve.Code, ve.Message)
	}
	return err.Error()
}
