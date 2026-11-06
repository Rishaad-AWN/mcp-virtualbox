package tools

import (
	"context"
	"os"
	"regexp"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/Rishaad-AWN/mcp-virtualbox/internal/vbox"
)

func registerDisplay(s *mcp.Server, d *deps) {
	add(s, "vm_screenshot",
		"Take a screenshot of the VM display, returns as image. Use this in a vision loop during OS "+
			"installation: screenshot -> analyze -> send input -> repeat.",
		object([]string{"vm"}, p("vm", str("VM name or UUID"))),
		func(ctx context.Context, in struct {
			VM string `json:"vm"`
		}) (*mcp.CallToolResult, error) {
			// Unique temp file avoids collisions between concurrent screenshots.
			f, err := os.CreateTemp(d.vb.Config().ScreenshotDir, "vbox_screenshot_*.png")
			if err != nil {
				return errText("failed to create temp file: " + err.Error()), nil
			}
			path := f.Name()
			f.Close()
			defer os.Remove(path)

			if _, err := d.vb.Run(ctx, "controlvm", in.VM, "screenshotpng", path); err != nil {
				return fail(err), nil
			}
			data, err := os.ReadFile(path)
			if err != nil {
				return errText("failed to read screenshot: " + err.Error()), nil
			}
			// ImageContent.Data is raw bytes; the SDK base64-encodes on the wire.
			return &mcp.CallToolResult{Content: []mcp.Content{
				&mcp.ImageContent{Data: data, MIMEType: "image/png"},
			}}, nil
		})

	add(s, "vm_screen_info", "Get VM display resolution and settings",
		object([]string{"vm"}, p("vm", str("VM name or UUID"))),
		func(ctx context.Context, in struct {
			VM string `json:"vm"`
		}) (*mcp.CallToolResult, error) {
			out, err := d.vb.Run(ctx, "showvminfo", in.VM, "--machinereadable")
			if err != nil {
				return fail(err), nil
			}
			info := vbox.ParseMachineReadable(out)

			width, height, bpp := "unknown", "unknown", "unknown"
			if m := videoModeRe.FindStringSubmatch(info["VideoMode"]); m != nil {
				width, height, bpp = m[1], m[2], m[3]
			}
			return jsonText(map[string]string{
				"width":              width,
				"height":             height,
				"bpp":                bpp,
				"vram":               firstNonEmpty(info["vram"], info["VRAMSize"]),
				"graphicsController": firstNonEmpty(info["graphicscontroller"], info["GraphicsControllerType"]),
				"monitors":           info["monitorcount"],
			}), nil
		})
}

// VideoMode looks like "720,400,0"@0,0 1 (width,height,bpp @ x,y screen).
var videoModeRe = regexp.MustCompile(`(\d+),(\d+),(\d+)`)

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}
