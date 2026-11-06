package tools

import (
	"context"
	"fmt"
	"net"
	"regexp"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/Rishaad-AWN/mcp-virtualbox/internal/vbox"
)

// vmIsActive reports whether the VM is running or paused (so NIC changes must go
// through controlvm rather than modifyvm).
func vmIsActive(ctx context.Context, vb *vbox.Client, vm string) bool {
	out, err := vb.Run(ctx, "showvminfo", vm, "--machinereadable")
	if err != nil {
		return false
	}
	state := vbox.ParseMachineReadable(out)["VMState"]
	return state == "running" || state == "paused"
}

func registerNetConfig(s *mcp.Server, d *deps) {
	add(s, "vm_set_nic",
		"Set a network adapter's attachment mode (nat, bridged, intnet, hostonly, natnetwork, none). "+
			"For bridged/intnet/hostonly/natnetwork, pass 'name' to select the host interface or network "+
			"(use vm_list_host_networks to find valid names). Works on running VMs (via controlvm) and "+
			"stopped VMs (via modifyvm).",
		object([]string{"vm", "mode"},
			p("vm", str("VM name or UUID")),
			p("adapter", def(integer("Network adapter number (1-based)"), 1)),
			p("mode", enum("Attachment mode", "nat", "bridged", "intnet", "hostonly", "natnetwork", "none")),
			p("name", str("Target name: host interface for bridged/hostonly, network name for intnet/natnetwork. Ignored for nat/none.")),
		),
		func(ctx context.Context, in struct {
			VM      string `json:"vm"`
			Adapter int    `json:"adapter"`
			Mode    string `json:"mode"`
			Name    string `json:"name"`
		}) (*mcp.CallToolResult, error) {
			n := in.Adapter
			if n < 1 {
				n = 1
			}
			vboxMode := in.Mode
			if vboxMode == "none" {
				vboxMode = "null"
			}

			if vmIsActive(ctx, d.vb, in.VM) {
				args := []string{"controlvm", in.VM, fmt.Sprintf("nic%d", n), vboxMode}
				if in.Name != "" && vboxMode != "nat" && vboxMode != "null" {
					args = append(args, in.Name)
				}
				if _, err := d.vb.Run(ctx, args...); err != nil {
					return fail(err), nil
				}
			} else {
				if _, err := d.vb.Run(ctx, "modifyvm", in.VM, fmt.Sprintf("--nic%d", n), vboxMode); err != nil {
					return fail(err), nil
				}
				if in.Name != "" {
					var flag string
					switch in.Mode {
					case "bridged":
						flag = fmt.Sprintf("--bridgeadapter%d", n)
					case "intnet":
						flag = fmt.Sprintf("--intnet%d", n)
					case "hostonly":
						flag = fmt.Sprintf("--hostonlyadapter%d", n)
					case "natnetwork":
						flag = fmt.Sprintf("--nat-network%d", n)
					}
					if flag != "" {
						if _, err := d.vb.Run(ctx, "modifyvm", in.VM, flag, in.Name); err != nil {
							return fail(err), nil
						}
					}
				}
			}
			target := ""
			if in.Name != "" {
				target = " -> " + in.Name
			}
			return text(fmt.Sprintf("NIC %d on %q set to %s%s.", n, in.VM, in.Mode, target)), nil
		})

	add(s, "vm_list_host_networks",
		"List host networking options available for VM adapters: bridged host interfaces, host-only "+
			"networks, and NAT networks. Use before vm_set_nic to pick a valid 'name'.",
		object(nil),
		func(ctx context.Context, _ struct{}) (*mcp.CallToolResult, error) {
			bridged, _ := d.vb.Run(ctx, "list", "bridgedifs")
			hostonly, _ := d.vb.Run(ctx, "list", "hostonlyifs")
			natnets, _ := d.vb.Run(ctx, "list", "natnetworks")
			return jsonText(map[string][]string{
				"bridged":  parseNamedList(bridged, "Name"),
				"hostonly": parseNamedList(hostonly, "Name"),
				// VirtualBox 6 labels them "NetworkName:", 7.x "Name:"
				"natnetworks": append(parseNamedList(natnets, "NetworkName"), parseNamedList(natnets, "Name")...),
			}), nil
		})

	add(s, "vm_nic_cable",
		"Connect or disconnect a network adapter's virtual cable (link state). Useful to simulate "+
			"network outages or isolate a VM without changing its attachment mode.",
		object([]string{"vm", "connected"},
			p("vm", str("VM name or UUID")),
			p("adapter", def(integer("Network adapter number (1-based)"), 1)),
			p("connected", boolean("true to plug the cable in, false to unplug it")),
		),
		func(ctx context.Context, in struct {
			VM        string `json:"vm"`
			Adapter   int    `json:"adapter"`
			Connected bool   `json:"connected"`
		}) (*mcp.CallToolResult, error) {
			n := in.Adapter
			if n < 1 {
				n = 1
			}
			onoff := "off"
			if in.Connected {
				onoff = "on"
			}
			var err error
			if vmIsActive(ctx, d.vb, in.VM) {
				_, err = d.vb.Run(ctx, "controlvm", in.VM, fmt.Sprintf("setlinkstate%d", n), onoff)
			} else {
				_, err = d.vb.Run(ctx, "modifyvm", in.VM, fmt.Sprintf("--cableconnected%d", n), onoff)
			}
			if err != nil {
				return fail(err), nil
			}
			state := "disconnected"
			if in.Connected {
				state = "connected"
			}
			return text(fmt.Sprintf("NIC %d cable on %q %s.", n, in.VM, state)), nil
		})
	add(s, "vm_create_host_network",
		"Create a network for lab VMs. type=natnetwork: a NAT network shared by several VMs, with internet "+
			"access and optional DHCP (needs name + network CIDR). type=hostonly: a host-only adapter so the "+
			"host and VMs can reach each other (optional ip/netmask; on Windows this adds a network adapter "+
			"to the host and may need admin rights). Attach VMs with vm_set_nic.",
		object([]string{"type"},
			p("type", enum("Network type", "natnetwork", "hostonly")),
			p("name", str("natnetwork: network name, e.g. LabNet")),
			p("network", str("natnetwork: network in CIDR form, e.g. 10.10.0.0/24")),
			p("dhcp", def(boolean("natnetwork: run VirtualBox's DHCP server on it"), true)),
			p("ip", str("hostonly: host address on the new adapter, e.g. 192.168.60.1")),
			p("netmask", def(str("hostonly: netmask for ip"), "255.255.255.0")),
		),
		func(ctx context.Context, in struct {
			Type    string `json:"type"`
			Name    string `json:"name"`
			Network string `json:"network"`
			DHCP    bool   `json:"dhcp"`
			IP      string `json:"ip"`
			Netmask string `json:"netmask"`
		}) (*mcp.CallToolResult, error) {
			if in.Type == "natnetwork" {
				if in.Name == "" {
					return errText("name is required for a natnetwork."), nil
				}
				if _, _, err := net.ParseCIDR(in.Network); err != nil {
					return errText(fmt.Sprintf("network must be CIDR like 10.10.0.0/24, got %q.", in.Network)), nil
				}
				dhcp := map[bool]string{true: "on", false: "off"}[in.DHCP]
				if _, err := d.vb.Run(ctx, "natnetwork", "add", "--netname="+in.Name, "--network="+in.Network,
					"--enable", "--dhcp="+dhcp); err != nil {
					return fail(err), nil
				}
				return text(fmt.Sprintf("NAT network %q (%s, DHCP %s) created. Attach VMs with vm_set_nic "+
					"mode=natnetwork name=%s.", in.Name, in.Network, dhcp, in.Name)), nil
			}
			if in.IP != "" && (net.ParseIP(in.IP) == nil || net.ParseIP(in.Netmask) == nil) {
				return errText(fmt.Sprintf("ip and netmask must be IPv4 addresses, got %q / %q.", in.IP, in.Netmask)), nil
			}
			out, err := d.vb.Run(ctx, "hostonlyif", "create")
			if err != nil {
				existing, _ := d.vb.Run(ctx, "list", "hostonlyifs")
				return errText(fmt.Sprintf("VirtualBox could not create a host-only adapter: %s. On Windows this "+
					"needs Administrator rights, and on some Windows builds VirtualBox fails here even then; try "+
					"VirtualBox Manager > Network > Host-only Networks > Create. Existing host-only adapters you can "+
					"use with vm_set_nic: %q.", vbox.FormatError(err), parseNamedList(existing, "Name"))), nil
			}
			name := createdHostOnlyName(out)
			if name == "" {
				return errText("Host-only adapter created but its name was not in VBoxManage's output: " + out), nil
			}
			if in.IP != "" {
				if _, err := d.vb.Run(ctx, "hostonlyif", "ipconfig", name, "--ip="+in.IP, "--netmask="+in.Netmask); err != nil {
					return errText(fmt.Sprintf("Created %q but setting its address failed: %s", name, vbox.FormatError(err))), nil
				}
			}
			return text(fmt.Sprintf("Host-only adapter %q created. Attach VMs with vm_set_nic mode=hostonly name=%q.", name, name)), nil
		})
}

var hostOnlyCreatedRe = regexp.MustCompile(`Interface '([^']+)' was successfully created`)

// createdHostOnlyName pulls the new adapter's name out of `VBoxManage hostonlyif create`.
func createdHostOnlyName(out string) string {
	if m := hostOnlyCreatedRe.FindStringSubmatch(out); m != nil {
		return m[1]
	}
	return ""
}

// parseNamedList extracts the value after "<key>:" from each matching line of
// `VBoxManage list` output (e.g. key="Name" for bridgedifs/hostonlyifs,
// key="NetworkName" for natnetworks).
func parseNamedList(output, key string) []string {
	re := regexp.MustCompile(`(?m)^` + regexp.QuoteMeta(key) + `:\s+(.+?)\s*$`)
	seen := map[string]bool{}
	var out []string
	for _, m := range re.FindAllStringSubmatch(output, -1) {
		v := m[1]
		if !seen[v] {
			seen[v] = true
			out = append(out, v)
		}
	}
	return out
}
