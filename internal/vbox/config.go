package vbox

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"time"
)

// Config holds runtime configuration for the VirtualBox MCP server. Values are
// resolved from environment variables with sensible cross-platform defaults.
type Config struct {
	// VBoxManagePath is the path to the VBoxManage executable.
	VBoxManagePath string
	// ScreenshotDir is where transient screenshot PNGs are written before being
	// read back and returned to the caller.
	ScreenshotDir string
	// DefaultTimeout bounds helper operations (e.g. the mouse web-service call).
	DefaultTimeout time.Duration
	// DryRun, when true, logs VBoxManage commands instead of executing them.
	DryRun bool
	// Debug, when true, logs every VBoxManage invocation to stderr.
	Debug bool
	// ReadOnly, when true, disables every tool that can modify a VM or its
	// guest (create/delete/modify/start/stop/input/exec/...), leaving only
	// observation tools. A safety valve for pointing an agent at real VMs.
	ReadOnly bool
}

// Load builds a Config from the environment (VBOXMANAGE_PATH, SCREENSHOT_DIR,
// DEFAULT_TIMEOUT, VBOXMANAGE_DRY_RUN, VBOXMANAGE_DEBUG).
func Load() *Config {
	return &Config{
		VBoxManagePath: findVBoxManage(),
		ScreenshotDir:  screenshotDir(),
		DefaultTimeout: envDuration("DEFAULT_TIMEOUT", 30*time.Second),
		DryRun:         os.Getenv("VBOXMANAGE_DRY_RUN") == "true",
		Debug:          os.Getenv("VBOXMANAGE_DEBUG") == "true",
		ReadOnly:       os.Getenv("VBOX_READONLY") == "true",
	}
}

// Logf writes a debug line to stderr when Debug is enabled. stderr is used
// because stdout carries the MCP protocol.
func (c *Config) Logf(format string, args ...any) {
	if c.Debug {
		fmt.Fprintf(os.Stderr, format+"\n", args...)
	}
}

func findVBoxManage() string {
	if p := os.Getenv("VBOXMANAGE_PATH"); p != "" {
		return p
	}

	var candidates []string
	if runtime.GOOS == "windows" {
		pf := os.Getenv("PROGRAMFILES")
		if pf == "" {
			pf = `C:\Program Files`
		}
		pf86 := os.Getenv("PROGRAMFILES(X86)")
		if pf86 == "" {
			pf86 = `C:\Program Files (x86)`
		}
		candidates = []string{
			filepath.Join(pf, "Oracle", "VirtualBox", "VBoxManage.exe"),
			filepath.Join(pf86, "Oracle", "VirtualBox", "VBoxManage.exe"),
			`C:\Program Files\Oracle\VirtualBox\VBoxManage.exe`,
		}
	} else {
		candidates = []string{
			"/usr/bin/VBoxManage",
			"/usr/local/bin/VBoxManage",
			"/opt/VirtualBox/VBoxManage",
		}
	}

	for _, c := range candidates {
		if fileExists(c) {
			return c
		}
	}
	// Fallback: hope it's on PATH.
	if runtime.GOOS == "windows" {
		return "VBoxManage.exe"
	}
	return "VBoxManage"
}

func screenshotDir() string {
	if d := os.Getenv("SCREENSHOT_DIR"); d != "" {
		return d
	}
	if d := os.Getenv("TEMP"); d != "" {
		return d
	}
	return os.TempDir()
}

func envDuration(key string, def time.Duration) time.Duration {
	if v := os.Getenv(key); v != "" {
		if ms, err := strconv.Atoi(v); err == nil && ms > 0 {
			return time.Duration(ms) * time.Millisecond
		}
	}
	return def
}

func fileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}
