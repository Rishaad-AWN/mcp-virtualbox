# CLAUDE.md — mcp-virtualbox

This file guides Claude Code when working in this repository. Read it fully before making any changes.

## Project Overview

A **Go MCP server** that gives AI assistants full control over Oracle VirtualBox VMs. It wraps the `VBoxManage` CLI and surfaces it as 50 MCP tools. It supports a vision-loop workflow (screenshot + keyboard/mouse) for OS installation, then efficient text-based control (Guest Additions / SSH / WinRM) once the OS is running.

- **Module:** `github.com/Rishaad-AWN/mcp-virtualbox`
- **Binary:** `mcp-virtualbox` (single static executable)
- **Transport:** stdio
- **MCP SDK:** `github.com/modelcontextprotocol/go-sdk` (official)

## Architecture

```
main.go                      # entry: load config, build server, register tools, run stdio
internal/
├── vbox/
│   ├── config.go            # Config + VBoxManage path auto-detection (env-driven)
│   ├── vbox.go              # Client.Run: the ONLY place VBoxManage is executed
│   └── parse.go             # ParseMachineReadable, ParseVMList
├── scancode/
│   └── scancode.go          # PS/2 Set 1 scancode table + Resolve("ctrl+alt+del")
├── hostnet/
│   └── hostnet.go           # host ARP table (arp -a / ip neigh) for MAC -> IP
├── hostclip/
│   └── hostclip.go          # set the HOST clipboard (Linux/macOS/Windows) for vm_paste_text
├── vboxweb/
│   └── mouse.go             # SOAP client for absolute mouse via vboxwebsrv
└── tools/
    ├── schema.go            # schema builders + result helpers + add[In]() registrar
    ├── register.go          # RegisterAll wires every group; VBOX_READONLY middleware
    ├── lifecycle.go         # core lifecycle tools + vm_wait_for_state (waitFor poller)
    ├── lifecycle_ext.go     # vm_clone, vm_export, vm_import, vm_rename
    ├── install.go           # vm_unattended_install, vm_disk_resize
    ├── guestip.go           # guest IP lookup: Guest Additions -> DHCP leases -> host ARP
    ├── display.go           # vm_screenshot, vm_screen_info
    ├── input.go             # keyboard + mouse (click/move/scroll/drag) + vm_paste_text
    ├── guest.go             # vm_guest_exec, vm_guest_copy_to/from
    ├── network.go           # SSH, WinRM, guest IP, waiters, NAT port forward
    ├── hostkey.go           # SSH trust-on-first-use host keys (own known_hosts file)
    ├── features.go          # vm_logs, vm_wait_for_port, vm_remove_nat_port_forward, vm_serial_setup/exec
    ├── netconfig.go         # vm_set_nic, vm_list_host_networks, vm_create_host_network, vm_nic_cable
    └── knowledge.go         # vm_install_guide (XP, 7, 10/11, Ubuntu)

scripts/mcp-virtualbox-launch      # POSIX plugin launcher (extensionless): PATH binary or download prebuilt
scripts/mcp-virtualbox-launch.cmd  # Windows plugin launcher (same, downloads the .zip)
scripts/version.sh                 # check|set version across main.go + manifests
.goreleaser.yaml + .github/workflows/release.yml  # cross-platform release on tag (runs version.sh check)
```

## Release process

1. `make bump v=X.Y.Z` — stamps `main.go` + `.claude-plugin/*.json`.
2. Update `CHANGELOG.md`; commit.
3. `git tag vX.Y.Z && git push --tags` — the release workflow runs `scripts/version.sh check` (fails if manifests ≠ tag), tests, then GoReleaser publishes cross-platform binaries. Always tag after a feature or `go install @latest` ships a stale binary.

**Key invariant:** every VBoxManage call goes through `vbox.Client.Run`. Never call `os/exec` directly from a tool file — the wrapper centralises dry-run, debug logging, `\r\n` normalisation and error formatting.

## MCP tool conventions

- **Registration:** use the `add[In]` helper in `schema.go`. It takes a name, description, a hand-built `*jsonschema.Schema`, and a `func(ctx, In) (*mcp.CallToolResult, error)`.
- **Schemas are hand-built** (not struct-tag inferred) so long descriptions can be plain Go strings with quotes/backslashes, and so defaults (`def(...)`) are applied by the SDK before the handler runs. Property names in the schema must match the `json` tags on the `In` struct.
- **Optional-with-default** params: list them out of the `required` slice and wrap the schema in `def(schema, value)`. The SDK fills the value in.
- **Optional-no-default** params (e.g. `memoryMB`): omit from `required`, no `def`; treat the zero value as "unset" in the handler.
- **Results:** `text()`, `jsonText()`, `errText()` (sets IsError), `fail(err)` (formats a VBox error). Screenshots return `mcp.ImageContent{Data: rawPNGBytes}` — the SDK base64-encodes on the wire.
- **Errors:** convert VBoxManage failures with `vbox.FormatError` → `[VBOX_E_*] message`. Never leak raw stderr.

## Key design decisions

1. **VBoxManage CLI only** — no COM/XPCOM bindings; everything via `os/exec` with argument arrays (no shell, no injection).
2. **Absolute mouse via web service** — `VBoxManage` has no mouse command. `internal/vboxweb` speaks SOAP to `vboxwebsrv`. The service is **started automatically on demand** (`ensureStarted`): on a transport error it sets `websrvauthlibrary null` (only when no `VBOXWEB_USER` is set), spawns a detached `vboxwebsrv`, waits for the port, and retries once. Opt out with `VBOXWEB_AUTOSTART=false`. Platform-specific process detaching lives in `detach_unix.go` / `detach_windows.go`.
3. **WinRM via `masterzen/winrm`** — supports Basic and NTLM auth.
4. **SSH via `golang.org/x/crypto/ssh`** — trust-on-first-use host keys stored in `<UserConfigDir>/mcp-virtualbox/known_hosts` (override `VBOX_MCP_KNOWN_HOSTS`); a changed key is refused unless the call sets `acceptNewHostKey=true`.
5. **USB tablet mouse** — `vm_create` sets `--mouse usbtablet` for absolute positioning matching screenshot pixels.
6. **Legacy OS detection** — `vm_create` auto-uses IDE (not SATA) for XP/2000/98/DOS, which lack AHCI drivers.
7. **`vm_create` rolls back** a half-created VM if a later step fails so no partial VM is left registered.
8. **`vm_list` reports precise state** (running/paused/saved/aborted/poweroff) via concurrent `showvminfo`.
9. **Read-only mode** — `VBOX_READONLY=true` installs `AddReceivingMiddleware(readOnlyMiddleware)` that rejects any `tools/call` not in `readOnlySafeTools`. One choke point; keep that allow-list in sync when adding observation tools.
10. **Mouse primitives share one SOAP session** — `vboxweb.withMouse` establishes logon→lock→getMouse and hands a `send(x,y,dz,dw,buttons)` closure to Click/Move/Scroll/Drag; `do()` wraps them with the autostart-retry.
11. **Release via GoReleaser on tag** — `go install @latest` follows the highest tag, so ALWAYS tag after a feature (an untagged feature means a stale binary).

## Build & test

```bash
make build     # go build -> ./mcp-virtualbox
make test      # go test ./...
make check     # gofmt -l, go vet, go test
go run . --help
```

Tests cover pure logic (parsers, scancodes, OS detection, guide lookup) and the SOAP mouse flow (via httptest). VBoxManage-dependent tools are exercised manually or in dry-run mode (`VBOXMANAGE_DRY_RUN=true`).

## Critical gotchas (embedded in tools + install guides)

| Gotcha | Where |
|--------|-------|
| Windows XP/2000 have NO SATA drivers — must use IDE | `isLegacyOS` in lifecycle.go; `vm_install_guide` |
| Windows blank-password accounts can't use guestcontrol | `vm_guest_exec` description; install guides |
| VBoxManage guestcontrol breaks args with spaces | `vm_guest_exec` args description; install guides |
| VBoxManage outputs `\r\n` | normalised in `vbox.Client.Run` |
| `--machinereadable` escapes backslashes as `\\` | unescaped in `ParseMachineReadable` |
| Mouse needs `vboxwebsrv` + `websrvauthlibrary null` | `vm_mouse_click` error message; README |
| Secrets never go on the command line (process list, debug log) | `secretFile` -> `--passwordfile=` / `--password-file=` in guest.go, install.go |
| `list natnetworks` says `NetworkName:` on VBox 6 but `Name:` on 7 | `vm_list_host_networks` reads both |
| VBox NAT port forwards accept the host side even while the guest is booting | `vm_wait_for_port` needs a banner by default |
| A tcpserver serial port allows one client at a time | `vm_serial_exec` error message |

## Adding a tool

1. Pick the right `internal/tools/*.go` group (or add one and call it from `register.go`).
2. Register with `add(s, "vm_x", "description", object([]string{...required...}, p("field", schema)...), handler)`.
3. Route all VBoxManage through `d.vb.Run(ctx, ...)`.
4. Add a unit test for any non-trivial parsing/logic.
5. Update the tool table in README.md and this file.
