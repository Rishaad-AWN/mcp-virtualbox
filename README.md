# mcp-virtualbox

Control your [VirtualBox](https://www.virtualbox.org/) virtual machines by chatting with an AI assistant.

mcp-virtualbox is an [MCP](https://modelcontextprotocol.io/) server: a small program that connects an AI assistant such as Claude to VirtualBox. Once it is connected you can say *"create an Ubuntu VM with 4 GB of RAM, install the OS, then set up SSH and show me its IP"*, and the assistant does the work for you, from creating the VM to typing inside it.

It is a single executable with no dependencies. There is no Python, Node.js or runtime to install.

## What you can do

- **Create and manage VMs:** create, clone, rename, import, export and delete VMs. Change their RAM, CPUs and disks, and take or restore snapshots.
- **Install operating systems:** hands-free unattended installs of Ubuntu, Debian, Fedora, RHEL-family and Windows 7+ from an ISO, or any other OS by watching the screen.
- **See and use the screen:** take screenshots, type, press key combinations, click, drag, scroll and paste text, so the assistant can work through any installer or desktop.
- **Run commands inside guests:** through Guest Additions, SSH or WinRM once the OS is running.
- **Use serial consoles:** type commands into routers and appliances (MikroTik CHR, VyOS, pfSense) or Linux, and read the text output, with no screenshots needed.
- **Set up networking:** NAT, bridged, host-only, internal and NAT networks, port forwarding, unplugging a virtual cable, and finding a guest's IP even without Guest Additions.
- **Troubleshoot:** read and filter a VM's log, wait for a VM to reach a state, or wait for a service such as SSH to come up.
- **Stay safe:** a read-only mode, clean shutdowns by default, and passwords that never appear on the command line.

### Example prompts

- *"List my VirtualBox VMs and start the one called Lab1."*
- *"Create an Ubuntu VM called web1 with 2 CPUs and 4 GB RAM, and install Ubuntu from C:\ISOs\ubuntu.iso."*
- *"Add a port forward from host port 2222 to SSH on web1, wait for SSH, and run `uname -a`."*
- *"Set up a serial console on my MikroTik VM, log in and show `/ip address print`."*
- *"Take a snapshot of web1 called before-update, then shut it down cleanly."*
- *"Why won't R1 start? Check its log."*

## How it works

```
AI assistant  <--MCP (stdio)-->  mcp-virtualbox  <--VBoxManage-->  VirtualBox
                                       |
                                       +--Guest Additions / SSH / WinRM / serial-->  guest OS
                                       +--VirtualBox web service--------------------> mouse input
```

The assistant starts `mcp-virtualbox` in the background and calls its tools. VM management goes through VirtualBox's own `VBoxManage` command. Inside the guest it can work in two ways:

1. **Screen mode:** screenshot, look, then type or click, and repeat. It works with any OS and any installer.
2. **Text mode:** fast command execution through Guest Additions, SSH, WinRM or the serial console once the OS is running.

## Requirements

- [VirtualBox](https://www.virtualbox.org/) 7.x. `VBoxManage` is found automatically in the standard install location.
- An MCP client: Claude Desktop, Claude Code, Google Antigravity, Cursor, VS Code with Copilot, Gemini CLI, OpenAI Codex, or any other client that supports local (stdio) servers.
- On Linux, your user must be in the `vboxusers` group.

Some tools need extra pieces:

| Feature | Needs |
|---|---|
| Mouse tools | The VirtualBox web service (`vboxwebsrv`), which ships with VirtualBox and is started automatically. See [Mouse](#mouse). |
| `vm_guest_exec`, file copy, clipboard paste | Guest Additions running in the guest |
| `vm_paste_text` on a Linux host | `xclip` or `xsel` (macOS and Windows need nothing extra) |
| `vm_exec_ssh` / `vm_exec_winrm` | An SSH or WinRM server enabled in the guest |
| `vm_serial_exec` | A guest that uses the serial console (routers do; Linux needs `console=ttyS0`) |

## Install

**Option 1: download (easiest)**

Get the archive for your system from [Releases](https://github.com/Rishaad-AWN/mcp-virtualbox/releases) and extract it:

| System | File |
|---|---|
| Windows | `mcp-virtualbox_windows_amd64.zip` |
| Windows on ARM | `mcp-virtualbox_windows_arm64.zip` |
| macOS (Apple Silicon) | `mcp-virtualbox_darwin_arm64.tar.gz` |
| macOS (Intel) | `mcp-virtualbox_darwin_amd64.tar.gz` |
| Linux | `mcp-virtualbox_linux_amd64.tar.gz` |
| Linux on ARM | `mcp-virtualbox_linux_arm64.tar.gz` |

Put the executable somewhere permanent, for example `C:\mcp\mcp-virtualbox.exe`.

**Option 2: with Go 1.25+**

```bash
go install github.com/Rishaad-AWN/mcp-virtualbox@latest
```

**Option 3: build from source**

```bash
git clone https://github.com/Rishaad-AWN/mcp-virtualbox
cd mcp-virtualbox
go build -o mcp-virtualbox.exe .
```

**Option 4: Claude Code plugin (downloads the right binary for you)**

```
/plugin marketplace add Rishaad-AWN/mcp-virtualbox
/plugin install mcp-virtualbox@mcp-virtualbox
```

## Connect it to your AI assistant

The examples below use `C:\mcp\mcp-virtualbox.exe`. Replace it with your own path. On macOS or Linux use a path like `/usr/local/bin/mcp-virtualbox`. In JSON files, Windows backslashes must be doubled (`C:\\mcp\\mcp-virtualbox.exe`).

After adding it, restart the app (or reload its MCP servers), then try: *"List my VirtualBox VMs"*.

### Claude Desktop

Settings → Developer → Edit Config opens `claude_desktop_config.json`. Add:

```json
{
  "mcpServers": {
    "virtualbox": {
      "command": "C:\\mcp\\mcp-virtualbox.exe"
    }
  }
}
```

### Claude Code

```bash
claude mcp add virtualbox -s user -- C:\mcp\mcp-virtualbox.exe
```

### Google Antigravity

In the agent panel open **… → MCP Servers → Manage MCP Servers → View raw config**. This edits `~/.gemini/config/mcp_config.json` (on Windows `C:\Users\<you>\.gemini\config\mcp_config.json`). Add:

```json
{
  "mcpServers": {
    "virtualbox": {
      "command": "C:\\mcp\\mcp-virtualbox.exe"
    }
  }
}
```

### Cursor

Edit `~/.cursor/mcp.json` (all projects) or `.cursor/mcp.json` (one project):

```json
{
  "mcpServers": {
    "virtualbox": {
      "type": "stdio",
      "command": "C:\\mcp\\mcp-virtualbox.exe"
    }
  }
}
```

### VS Code (GitHub Copilot)

Run **MCP: Open User Configuration** from the Command Palette, or create `.vscode/mcp.json` in your project. Note that VS Code uses `servers`, not `mcpServers`:

```json
{
  "servers": {
    "virtualbox": {
      "type": "stdio",
      "command": "C:\\mcp\\mcp-virtualbox.exe"
    }
  }
}
```

### Gemini CLI

```bash
gemini mcp add -s user virtualbox C:\mcp\mcp-virtualbox.exe
```

Or add the same `mcpServers` block as Claude Desktop to `~/.gemini/settings.json`.

### OpenAI Codex (CLI and IDE extension)

```bash
codex mcp add virtualbox -- C:\mcp\mcp-virtualbox.exe
```

Or add this to `~/.codex/config.toml`:

```toml
[mcp_servers.virtualbox]
command = 'C:\mcp\mcp-virtualbox.exe'
```

### Other MCP clients

Any client that can start a local (stdio) MCP server works. Point its command at the `mcp-virtualbox` executable. Most clients use the same `mcpServers` JSON shown for Claude Desktop.

### Settings

Everything works with no settings. These environment variables are optional:

| Variable | Purpose |
|---|---|
| `VBOXMANAGE_PATH` | Path to `VBoxManage`, if it is not found automatically |
| `VBOX_READONLY` | `true` to allow only tools that look, never change (see [Read-only mode](#read-only-mode)) |
| `DEFAULT_TIMEOUT` | Default operation timeout in ms (default 30000) |
| `SCREENSHOT_DIR` | Where temporary screenshots go (default: temp folder) |
| `VBOXMANAGE_DRY_RUN` | `true` to print VBoxManage commands instead of running them |
| `VBOXMANAGE_DEBUG` | `true` to log every VBoxManage command |
| `VBOXWEB_ENDPOINT` `VBOXWEB_USER` `VBOXWEB_PASSWORD` `VBOXWEB_AUTOSTART` `VBOXWEB_BIN` | Web service settings for the mouse tools |
| `VBOX_MCP_KNOWN_HOSTS` | File where SSH host keys are remembered |

Add them under `"env"` in the JSON config, for example `"env": { "VBOX_READONLY": "true" }`.

## Tools (50)

| Area | Tools |
|---|---|
| VMs | `vm_list` `vm_info` `vm_create` `vm_modify` `vm_delete` `vm_clone` `vm_rename` `vm_export` `vm_import` `vm_list_ostypes` |
| Power | `vm_start` `vm_stop` `vm_pause` `vm_resume` `vm_reset` `vm_wait_for_state` |
| Snapshots and media | `vm_snapshot` (take, restore, delete, list) `vm_attach_media` `vm_shared_folder_add` `vm_shared_folder_remove` |
| OS install and disks | `vm_unattended_install` `vm_disk_resize` `vm_install_guide` |
| Screen | `vm_screenshot` `vm_screen_info` |
| Keyboard and mouse | `vm_keyboard_type` `vm_keyboard_scancode` `vm_paste_text` `vm_mouse_click` `vm_mouse_move` `vm_mouse_scroll` `vm_mouse_drag` |
| Inside the guest | `vm_guest_exec` `vm_guest_copy_to` `vm_guest_copy_from` `vm_exec_ssh` `vm_exec_winrm` |
| Serial console | `vm_serial_setup` `vm_serial_exec` |
| Networking | `vm_set_nic` `vm_nic_cable` `vm_list_host_networks` `vm_create_host_network` `vm_add_nat_port_forward` `vm_remove_nat_port_forward` |
| Waiting and troubleshooting | `vm_get_guest_ip` `vm_wait_for_network` `vm_wait_for_guest_additions` `vm_wait_for_port` `vm_logs` |

## Good to know

### Clean shutdowns

`vm_stop` presses the virtual power button first, so the guest shuts down properly. It only cuts the power if the guest ignores the button for 60 seconds (adjust with `timeoutSec`). Paused, stuck or crashed VMs are powered off straight away. Ask for `method: poweroff` if you want an instant power cut.

### Typing long text

`vm_keyboard_type` simulates keystrokes and can drop characters in long or non-English text. `vm_paste_text` is more reliable: it puts the text on the clipboard and pastes it into the guest. It needs Guest Additions with clipboard sharing, and on a Linux host `xclip` or `xsel`.

### Serial console

`vm_serial_setup` (VM powered off) connects the VM's COM1 port to a TCP port on your machine. After you start the VM, `vm_serial_exec` types lines and returns what the console prints, including login prompts. Send the line `^C` for Ctrl+C. Only one client can use a serial console at a time.

### SSH host keys

The first time `vm_exec_ssh` connects to a VM it remembers that VM's key, and later connections must match it. If you reinstall or replace the VM, the key changes and the connection is refused. Call it again with `acceptNewHostKey: true` to trust the new key.

### Read-only mode

Set `VBOX_READONLY=true` to block every tool that changes something. Only these remain: `vm_list`, `vm_info`, `vm_screen_info`, `vm_screenshot`, `vm_list_ostypes`, `vm_install_guide`, `vm_get_guest_ip`, `vm_list_host_networks`, `vm_logs` and the `vm_wait_for_*` tools. This is useful when an assistant should only look at important VMs.

### Mouse

`VBoxManage` has no mouse command, so the mouse tools go through the VirtualBox web service (`vboxwebsrv`). The server starts it automatically the first time it is needed, so normally there is nothing to set up. To run it yourself, set `VBOXWEB_AUTOSTART=false` and start it with:

```bash
VBoxManage setproperty websrvauthlibrary null
vboxwebsrv --host 127.0.0.1 --port 18083
```

VMs made with `vm_create` already use a USB tablet mouse, which accurate clicking needs.

## Development

```bash
make build   # build the binary
make test    # go test ./...
make check   # gofmt check + go vet + tests
```

See [CLAUDE.md](CLAUDE.md) for architecture notes.

## License

MIT. See [LICENSE](LICENSE).
