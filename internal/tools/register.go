// Package tools registers all VirtualBox MCP tools on a server.
package tools

import (
	"context"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/Rishaad-AWN/mcp-virtualbox/internal/vbox"
	"github.com/Rishaad-AWN/mcp-virtualbox/internal/vboxweb"
)

// RegisterAll wires every tool group onto the server using the given VBoxManage
// client and web-service mouse client. In read-only mode it also installs
// middleware that blocks every mutating tool.
func RegisterAll(s *mcp.Server, vb *vbox.Client, mouse *vboxweb.Client) {
	d := &deps{vb: vb, mouse: mouse}
	registerLifecycle(s, d)
	registerLifecycleExt(s, d)
	registerDisplay(s, d)
	registerInput(s, d)
	registerGuest(s, d)
	registerNetwork(s, d)
	registerNetConfig(s, d)
	registerWaitForState(s, d)
	registerFeatures(s, d)
	registerKnowledge(s, d)
	registerInstall(s, d)

	if vb.Config().ReadOnly {
		s.AddReceivingMiddleware(readOnlyMiddleware)
	}
}

// readOnlySafeTools are the only tools allowed when VBOX_READONLY=true: pure
// observation, no VM/guest mutation.
var readOnlySafeTools = map[string]bool{
	"vm_list":                     true,
	"vm_info":                     true,
	"vm_screen_info":              true,
	"vm_screenshot":               true,
	"vm_list_ostypes":             true,
	"vm_install_guide":            true,
	"vm_get_guest_ip":             true,
	"vm_list_host_networks":       true,
	"vm_wait_for_guest_additions": true,
	"vm_wait_for_network":         true,
	"vm_wait_for_state":           true,
	"vm_logs":                     true,
	"vm_wait_for_port":            true,
}

// readOnlyMiddleware rejects any tools/call for a tool not in readOnlySafeTools,
// returning a tool error (not a protocol error) so the model sees a clear reason.
func readOnlyMiddleware(next mcp.MethodHandler) mcp.MethodHandler {
	return func(ctx context.Context, method string, req mcp.Request) (mcp.Result, error) {
		if method == "tools/call" {
			if p, ok := req.GetParams().(*mcp.CallToolParamsRaw); ok && !readOnlySafeTools[p.Name] {
				return &mcp.CallToolResult{
					IsError: true,
					Content: []mcp.Content{&mcp.TextContent{
						Text: "read-only mode (VBOX_READONLY=true): tool " + p.Name +
							" is disabled because it can modify a VM. Unset VBOX_READONLY to enable mutating tools.",
					}},
				}, nil
			}
		}
		return next(ctx, method, req)
	}
}
