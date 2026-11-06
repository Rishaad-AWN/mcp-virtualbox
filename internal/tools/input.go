package tools

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/Rishaad-AWN/mcp-virtualbox/internal/hostclip"
	"github.com/Rishaad-AWN/mcp-virtualbox/internal/scancode"
	"github.com/Rishaad-AWN/mcp-virtualbox/internal/vbox"
)

var hexByteRe = regexp.MustCompile(`^[0-9a-f]{1,2}$`)

func registerInput(s *mcp.Server, d *deps) {
	add(s, "vm_keyboard_scancode",
		"Send named key combinations as PS/2 scancodes (e.g. 'enter', 'ctrl+alt+delete', 'f8', 'tab'). "+
			"Use this for special keys, shortcuts, and navigation. Numeric-keypad keys are available as "+
			"kp_0..kp_9, kp_enter, kp_dot, kp_plus, kp_minus, kp_star, kp_slash. For keys not in the named "+
			"map, pass raw make/break bytes via 'raw'.",
		object([]string{"vm"},
			p("vm", str("VM name or UUID")),
			p("keys", str("Key combo like 'enter', 'tab', 'f8', 'ctrl+alt+delete', 'alt+f4', 'shift+a', 'kp_1'. Either 'keys' or 'raw' must be provided.")),
			p("raw", str("Raw PS/2 Set 1 scancode bytes as space-separated hex, sent verbatim (e.g. '4f cf' for keypad-1 make+break). Include both make and break bytes. Use when a key is not in the named map. Takes precedence over 'keys'.")),
			p("count", def(integer("Number of times to send the key combo"), 1)),
			p("delayMs", def(integer("Delay in ms between repeated key presses"), 50)),
		),
		func(ctx context.Context, in struct {
			VM      string `json:"vm"`
			Keys    string `json:"keys"`
			Raw     string `json:"raw"`
			Count   int    `json:"count"`
			DelayMs int    `json:"delayMs"`
		}) (*mcp.CallToolResult, error) {
			var codes []string
			var label string
			switch {
			case in.Raw != "":
				for _, b := range strings.Fields(in.Raw) {
					b = strings.ToLower(b)
					if !hexByteRe.MatchString(b) {
						return errText(fmt.Sprintf("Error: 'raw' must be space-separated hex bytes (e.g. '4f cf'). Invalid byte: %q", b)), nil
					}
					if len(b) == 1 {
						b = "0" + b
					}
					codes = append(codes, b)
				}
				if len(codes) == 0 {
					return errText("Error: 'raw' must be space-separated hex bytes (e.g. '4f cf')."), nil
				}
				label = "raw [" + strings.Join(codes, " ") + "]"
			case in.Keys != "":
				var err error
				codes, err = scancode.Resolve(in.Keys)
				if err != nil {
					return errText(fmt.Sprintf("Error: %s\n\nAvailable keys: %s", err.Error(), strings.Join(scancode.AvailableKeys(), ", "))), nil
				}
				label = fmt.Sprintf("%q", in.Keys)
			default:
				return errText("Error: provide either 'keys' (named combo) or 'raw' (hex scancode bytes)."), nil
			}

			count := in.Count
			if count < 1 {
				count = 1
			}
			for i := 0; i < count; i++ {
				if _, err := d.vb.Run(ctx, append([]string{"controlvm", in.VM, "keyboardputscancode"}, codes...)...); err != nil {
					return fail(err), nil
				}
				if i < count-1 && in.DelayMs > 0 {
					time.Sleep(time.Duration(in.DelayMs) * time.Millisecond)
				}
			}
			return text(fmt.Sprintf("Sent %s x%d (scancodes: %s)", label, count, strings.Join(codes, " "))), nil
		})

	add(s, "vm_keyboard_type",
		"Type printable text string into the VM (uses keyboardputstring). For special keys use "+
			"vm_keyboard_scancode instead. Long strings are chunked with an inter-chunk delay to avoid "+
			"overrunning the guest keyboard buffer (which silently drops characters).",
		object([]string{"vm", "text"},
			p("vm", str("VM name or UUID")),
			p("text", str("Text to type")),
			p("chunkSize", def(integer("Max characters sent per keyboardputstring call. Smaller is safer; the guest keyboard buffer overruns and drops characters if fed too much at once."), 50)),
			p("delayMs", def(integer("Delay in ms between chunks, giving the guest time to drain its keyboard buffer."), 30)),
		),
		func(ctx context.Context, in struct {
			VM        string `json:"vm"`
			Text      string `json:"text"`
			ChunkSize int    `json:"chunkSize"`
			DelayMs   int    `json:"delayMs"`
		}) (*mcp.CallToolResult, error) {
			size := in.ChunkSize
			if size < 1 {
				size = 1
			}
			runes := []rune(in.Text)
			for i := 0; i < len(runes); i += size {
				end := i + size
				if end > len(runes) {
					end = len(runes)
				}
				chunk := string(runes[i:end])
				if _, err := d.vb.Run(ctx, "controlvm", in.VM, "keyboardputstring", chunk); err != nil {
					return fail(err), nil
				}
				if end < len(runes) && in.DelayMs > 0 {
					time.Sleep(time.Duration(in.DelayMs) * time.Millisecond)
				}
			}
			return text(fmt.Sprintf("Typed %d characters.", len(runes))), nil
		})

	add(s, "vm_mouse_click",
		"Click at absolute screen coordinates (requires USB tablet mouse mode, enabled by vm_create). "+
			"Coordinates match screenshot pixel positions. Uses the VirtualBox web service (vboxwebsrv) for "+
			"absolute positioning - no Python required; the service is started automatically on demand.",
		object([]string{"vm", "x", "y"},
			p("vm", str("VM name or UUID")),
			p("x", integer("X coordinate (pixels from left)")),
			p("y", integer("Y coordinate (pixels from top)")),
			p("button", def(enum("Mouse button", "left", "right", "middle"), "left")),
			p("doubleClick", def(boolean("Double click"), false)),
		),
		func(ctx context.Context, in struct {
			VM          string `json:"vm"`
			X           int    `json:"x"`
			Y           int    `json:"y"`
			Button      string `json:"button"`
			DoubleClick bool   `json:"doubleClick"`
		}) (*mcp.CallToolResult, error) {
			if d.vb.Config().DryRun {
				return text(fmt.Sprintf("[dry-run] would click %s at (%d, %d)", in.Button, in.X, in.Y)), nil
			}
			if err := d.mouse.Click(ctx, in.VM, in.X, in.Y, in.Button, in.DoubleClick); err != nil {
				return errText("Mouse click failed: " + err.Error()), nil
			}
			suffix := ""
			if in.DoubleClick {
				suffix = " (double)"
			}
			return text(fmt.Sprintf("Clicked %s at (%d, %d)%s", in.Button, in.X, in.Y, suffix)), nil
		})

	add(s, "vm_mouse_move",
		"Move the mouse pointer to absolute screen coordinates without clicking (requires USB tablet "+
			"mouse mode, enabled by vm_create). Coordinates match screenshot pixel positions.",
		object([]string{"vm", "x", "y"},
			p("vm", str("VM name or UUID")),
			p("x", integer("X coordinate (pixels from left)")),
			p("y", integer("Y coordinate (pixels from top)")),
		),
		func(ctx context.Context, in struct {
			VM string `json:"vm"`
			X  int    `json:"x"`
			Y  int    `json:"y"`
		}) (*mcp.CallToolResult, error) {
			if d.vb.Config().DryRun {
				return text(fmt.Sprintf("[dry-run] would move to (%d, %d)", in.X, in.Y)), nil
			}
			if err := d.mouse.Move(ctx, in.VM, in.X, in.Y); err != nil {
				return errText("Mouse move failed: " + err.Error()), nil
			}
			return text(fmt.Sprintf("Moved to (%d, %d)", in.X, in.Y)), nil
		})

	add(s, "vm_mouse_scroll",
		"Scroll the mouse wheel at absolute screen coordinates. The pointer is moved to (x, y) first, "+
			"then the wheel is turned. Use for scrolling lists, pages, and dropdowns.",
		object([]string{"vm", "x", "y"},
			p("vm", str("VM name or UUID")),
			p("x", integer("X coordinate (pixels from left)")),
			p("y", integer("Y coordinate (pixels from top)")),
			p("direction", def(enum("Scroll direction", "up", "down", "left", "right"), "down")),
			p("amount", def(integer("Number of wheel notches to scroll"), 3)),
		),
		func(ctx context.Context, in struct {
			VM        string `json:"vm"`
			X         int    `json:"x"`
			Y         int    `json:"y"`
			Direction string `json:"direction"`
			Amount    int    `json:"amount"`
		}) (*mcp.CallToolResult, error) {
			amt := in.Amount
			if amt <= 0 {
				amt = 3
			}
			var dz, dw int
			switch in.Direction {
			case "up":
				dz = amt
			case "down":
				dz = -amt
			case "right":
				dw = amt
			case "left":
				dw = -amt
			}
			if d.vb.Config().DryRun {
				return text(fmt.Sprintf("[dry-run] would scroll %s x%d at (%d, %d)", in.Direction, amt, in.X, in.Y)), nil
			}
			if err := d.mouse.Scroll(ctx, in.VM, in.X, in.Y, dz, dw); err != nil {
				return errText("Mouse scroll failed: " + err.Error()), nil
			}
			return text(fmt.Sprintf("Scrolled %s x%d at (%d, %d)", in.Direction, amt, in.X, in.Y)), nil
		})

	add(s, "vm_mouse_drag",
		"Press a mouse button at a start position, drag to an end position while holding, then release. "+
			"Use for drag-select, moving windows/icons, and sliders. Coordinates match screenshot pixels.",
		object([]string{"vm", "startX", "startY", "endX", "endY"},
			p("vm", str("VM name or UUID")),
			p("startX", integer("Start X coordinate (pixels from left)")),
			p("startY", integer("Start Y coordinate (pixels from top)")),
			p("endX", integer("End X coordinate (pixels from left)")),
			p("endY", integer("End Y coordinate (pixels from top)")),
			p("button", def(enum("Mouse button to hold during the drag", "left", "right", "middle"), "left")),
		),
		func(ctx context.Context, in struct {
			VM     string `json:"vm"`
			StartX int    `json:"startX"`
			StartY int    `json:"startY"`
			EndX   int    `json:"endX"`
			EndY   int    `json:"endY"`
			Button string `json:"button"`
		}) (*mcp.CallToolResult, error) {
			if d.vb.Config().DryRun {
				return text(fmt.Sprintf("[dry-run] would drag %s from (%d, %d) to (%d, %d)", in.Button, in.StartX, in.StartY, in.EndX, in.EndY)), nil
			}
			if err := d.mouse.Drag(ctx, in.VM, in.StartX, in.StartY, in.EndX, in.EndY, in.Button); err != nil {
				return errText("Mouse drag failed: " + err.Error()), nil
			}
			return text(fmt.Sprintf("Dragged %s from (%d, %d) to (%d, %d)", in.Button, in.StartX, in.StartY, in.EndX, in.EndY)), nil
		})

	add(s, "vm_paste_text",
		"Enter text into the guest via the shared clipboard instead of simulated keystrokes: sets the host "+
			"clipboard, lets VirtualBox sync it to the guest, then sends a paste chord. More reliable than "+
			"vm_keyboard_type for long or Unicode text (which can overrun the guest keyboard buffer). "+
			"Requires: Guest Additions with clipboard sharing running in the guest, a running VM with the "+
			"target field focused, and a host clipboard helper. On Linux, VirtualBox reads the X11 "+
			"clipboard, so xclip or xsel is preferred (on Wayland sessions XWayland must be available); "+
			"macOS uses pbcopy, Windows uses PowerShell.",
		object([]string{"vm", "text"},
			p("vm", str("VM name or UUID")),
			p("text", str("Text to paste into the focused guest field")),
			p("pasteKey", def(str("Key chord that pastes in the guest (e.g. 'ctrl+v', or 'shift+insert' for Linux terminals)"), "ctrl+v")),
			p("setClipboardMode", def(boolean("Set the VM's shared-clipboard mode to bidirectional first (needs a running VM)"), true)),
			p("syncDelayMs", def(integer("Delay in ms after copying before pasting, to let Guest Additions sync the clipboard"), 400)),
		),
		func(ctx context.Context, in struct {
			VM               string `json:"vm"`
			Text             string `json:"text"`
			PasteKey         string `json:"pasteKey"`
			SetClipboardMode bool   `json:"setClipboardMode"`
			SyncDelayMs      int    `json:"syncDelayMs"`
		}) (*mcp.CallToolResult, error) {
			codes, err := scancode.Resolve(in.PasteKey)
			if err != nil {
				return errText(fmt.Sprintf("Error: invalid pasteKey %q: %s", in.PasteKey, err.Error())), nil
			}
			if d.vb.Config().DryRun {
				return text(fmt.Sprintf("[dry-run] would paste %d chars into %q via %q", len([]rune(in.Text)), in.VM, in.PasteKey)), nil
			}
			if in.SetClipboardMode {
				if _, err := d.vb.Run(ctx, "controlvm", in.VM, "clipboard", "mode", "bidirectional"); err != nil {
					return errText("Failed to enable shared clipboard (is the VM running?): " + vbox.FormatError(err)), nil
				}
			}
			if err := hostclip.Set(ctx, in.Text); err != nil {
				return errText("Failed to set host clipboard: " + err.Error()), nil
			}
			if in.SyncDelayMs > 0 {
				time.Sleep(time.Duration(in.SyncDelayMs) * time.Millisecond)
			}
			if _, err := d.vb.Run(ctx, append([]string{"controlvm", in.VM, "keyboardputscancode"}, codes...)...); err != nil {
				return fail(err), nil
			}
			return text(fmt.Sprintf("Pasted %d characters into %q via %q.", len([]rune(in.Text)), in.VM, in.PasteKey)), nil
		})
}
