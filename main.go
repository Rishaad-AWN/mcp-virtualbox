// Command mcp-virtualbox is a Model Context Protocol server that gives AI
// assistants full control over Oracle VirtualBox VMs: a vision loop
// (screenshot + keyboard/mouse) for OS installation, then fast text-based
// control via Guest Additions, SSH or WinRM once the OS is running.
//
// It ships as a single static binary with no Node.js or Python runtime dependency.
package main

import (
	"context"
	"fmt"
	"os"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/Rishaad-AWN/mcp-virtualbox/internal/tools"
	"github.com/Rishaad-AWN/mcp-virtualbox/internal/vbox"
	"github.com/Rishaad-AWN/mcp-virtualbox/internal/vboxweb"
)

// version is overridden at build time via -ldflags "-X main.version=...".
var version = "1.0.0"

func main() {
	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "-v", "--version", "version":
			fmt.Println("mcp-virtualbox " + version)
			return
		case "-h", "--help", "help":
			printHelp()
			return
		}
	}

	cfg := vbox.Load()
	vb := vbox.New(cfg)
	mouse := vboxweb.NewFromEnv(cfg.DefaultTimeout, cfg.VBoxManagePath)

	server := mcp.NewServer(&mcp.Implementation{
		Name:    "mcp-virtualbox",
		Version: version,
	}, nil)

	tools.RegisterAll(server, vb, mouse)

	fmt.Fprintln(os.Stderr, "mcp-virtualbox server running on stdio")
	if err := server.Run(context.Background(), &mcp.StdioTransport{}); err != nil {
		fmt.Fprintln(os.Stderr, "Fatal error:", err)
		os.Exit(1)
	}
}

func printHelp() {
	fmt.Print(`mcp-virtualbox - VirtualBox MCP server (stdio transport)

Usage:
  mcp-virtualbox            Run the MCP server on stdio (default)
  mcp-virtualbox --version  Print version and exit

Environment:
  VBOXMANAGE_PATH      Path to VBoxManage (auto-detected if unset)
  SCREENSHOT_DIR       Directory for transient screenshots (default: temp dir)
  DEFAULT_TIMEOUT      Default operation timeout in ms (default: 30000)
  VBOXMANAGE_DRY_RUN   "true" to log VBoxManage commands without running them
  VBOXMANAGE_DEBUG     "true" to log every VBoxManage invocation to stderr
  VBOX_READONLY        "true" to disable all mutating tools (observation only)
  VBOXWEB_ENDPOINT     VirtualBox web service URL (default: http://127.0.0.1:18083/)
  VBOXWEB_USER         Web service username (default: empty; use null auth)
  VBOXWEB_PASSWORD     Web service password (default: empty)
  VBOXWEB_AUTOSTART    "false" to disable auto-starting vboxwebsrv on demand
  VBOXWEB_BIN          Path to vboxwebsrv (default: next to VBoxManage, or PATH)
  VBOX_MCP_KNOWN_HOSTS File where vm_exec_ssh remembers guest host keys

Mouse control uses the VirtualBox web service, which is started automatically on
demand. To manage it yourself instead, set VBOXWEB_AUTOSTART=false and run:
  VBoxManage setproperty websrvauthlibrary null
  vboxwebsrv --host 127.0.0.1 --port 18083 &
`)
}
