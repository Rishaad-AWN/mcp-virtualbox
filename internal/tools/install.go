package tools

import (
	"context"
	"fmt"
	"os"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/Rishaad-AWN/mcp-virtualbox/internal/vbox"
)

func registerInstall(s *mcp.Server, d *deps) {
	add(s, "vm_unattended_install",
		"Install an OS hands-free from its ISO with VirtualBox's unattended installer (Ubuntu/Debian/Fedora/"+
			"RHEL-family and Windows 7+). Much faster and more reliable than driving the installer with "+
			"screenshots. Create the VM first with vm_create; it must be powered off. The VM boots and installs "+
			"by itself; follow it with vm_screenshot, then vm_wait_for_guest_additions when installAdditions=true.",
		object([]string{"vm", "iso", "username", "password"},
			p("vm", str("VM name or UUID (powered off)")),
			p("iso", str("Absolute host path to the OS installer ISO")),
			p("username", str("User account to create")),
			p("password", str("Password for that user (also the root/Administrator password). Not shown in process lists.")),
			p("fullName", str("User's full name (default: username)")),
			p("hostname", str("Guest hostname; '.local' is added if it has no domain (default: <vm>.local)")),
			p("installAdditions", def(boolean("Also install Guest Additions so vm_guest_exec / vm_get_guest_ip work"), true)),
			p("locale", str("Locale such as en_US (default: VirtualBox's guess from the host)")),
			p("timeZone", str("Time zone such as UTC or Asia/Jakarta (default: host's)")),
			p("imageIndex", integer("Windows only: edition index inside install.wim (see the detected images in the result)")),
			p("productKey", str("Windows only: product key")),
			p("startVM", def(enum("Start the VM afterwards to begin installing", "headless", "gui", "none"), "headless")),
		),
		func(ctx context.Context, in struct {
			VM               string `json:"vm"`
			ISO              string `json:"iso"`
			Username         string `json:"username"`
			Password         string `json:"password"`
			FullName         string `json:"fullName"`
			Hostname         string `json:"hostname"`
			InstallAdditions bool   `json:"installAdditions"`
			Locale           string `json:"locale"`
			TimeZone         string `json:"timeZone"`
			ImageIndex       int    `json:"imageIndex"`
			ProductKey       string `json:"productKey"`
			StartVM          string `json:"startVM"`
		}) (*mcp.CallToolResult, error) {
			if _, err := os.Stat(in.ISO); err != nil {
				return errText("ISO not found on the host: " + in.ISO), nil
			}
			if state, err := d.vmState(ctx, in.VM); err != nil {
				return fail(err), nil
			} else if state != "poweroff" && state != "aborted" {
				return errText(fmt.Sprintf("VM %q is %s; power it off before an unattended install.", in.VM, state)), nil
			}

			// detect exits non-zero for ISOs it cannot install; VirtualBox's own install error is just E_FAIL
			detected, derr := d.vb.Run(ctx, "unattended", "detect", "--iso="+in.ISO, "--machine-readable")
			det := vbox.ParseMachineReadable(detected)
			if derr != nil || det["IsInstallSupported"] == "off" {
				return errText("VirtualBox does not recognise " + in.ISO + " as an installer it can automate. " +
					"Use an official Ubuntu/Debian/Fedora/RHEL-family or Windows 7+ installer ISO, or install " +
					"manually with vm_attach_media + vm_screenshot + vm_keyboard_type."), nil
			}

			pwFile, cleanup, err := secretFile(in.Password)
			if err != nil {
				return errText(err.Error()), nil
			}
			defer cleanup()

			args := unattendedArgs(in.VM, in.ISO, in.Username, pwFile, in.FullName, in.Hostname,
				in.InstallAdditions, in.Locale, in.TimeZone, in.ImageIndex, in.ProductKey, in.StartVM)
			out, err := d.vb.Run(ctx, args...)
			if err != nil {
				return fail(err), nil
			}
			return jsonText(map[string]any{
				"status":   "install started",
				"vm":       in.VM,
				"started":  in.StartVM != "none",
				"detected": detectedSummary(det),
				"next":     "Watch with vm_screenshot. Installs take ~10-40 min and reboot on their own.",
				"output":   strings.TrimSpace(out),
			}), nil
		})

	add(s, "vm_disk_resize",
		"Grow a VM's virtual hard disk (VDI/VHD; VMDK only when dynamically allocated). Disks cannot shrink. "+
			"The VM must be powered off and should have no snapshots on that disk. Afterwards extend the "+
			"partition inside the guest (e.g. growpart + resize2fs on Linux, Disk Management on Windows).",
		object([]string{"vm", "sizeMB"},
			p("vm", str("VM name or UUID (powered off)")),
			p("sizeMB", integer("New total size in MB (must be larger than the current size)")),
			p("disk", str("Which disk: its path or UUID, or a slot like SATA-0-0 (default: the first hard disk)")),
		),
		func(ctx context.Context, in struct {
			VM     string `json:"vm"`
			SizeMB int    `json:"sizeMB"`
			Disk   string `json:"disk"`
		}) (*mcp.CallToolResult, error) {
			out, err := d.vb.Run(ctx, "showvminfo", in.VM, "--machinereadable")
			if err != nil {
				return fail(err), nil
			}
			info := vbox.ParseMachineReadable(out)
			if st := info["VMState"]; st != "poweroff" && st != "aborted" {
				return errText(fmt.Sprintf("VM %q is %s; power it off before resizing its disk.", in.VM, st)), nil
			}
			disk, ok := pickDisk(info, in.Disk)
			if !ok {
				return errText("No matching hard disk on this VM. Attached disks: " + strings.Join(diskSlots(info), ", ")), nil
			}
			hd, err := d.vb.Run(ctx, "showmediuminfo", "disk", disk.uuid)
			if err != nil {
				return fail(err), nil
			}
			cur := mediumCapacityMB(hd)
			if cur > 0 && in.SizeMB <= cur {
				return errText(fmt.Sprintf("Disk is already %d MB; the new size must be larger (disks cannot shrink).", cur)), nil
			}
			if _, err := d.vb.Run(ctx, "modifymedium", "disk", disk.uuid, "--resize", strconv.Itoa(in.SizeMB)); err != nil {
				return fail(err), nil
			}
			return text(fmt.Sprintf("Disk %s (%s) resized from %d MB to %d MB. Extend the partition inside the guest to use the space.",
				disk.slot, disk.path, cur, in.SizeMB)), nil
		})
}

func unattendedArgs(vm, iso, user, pwFile, fullName, hostname string, additions bool,
	locale, tz string, imageIndex int, productKey, startVM string) []string {
	if fullName == "" {
		fullName = user
	}
	if hostname == "" {
		hostname = vm
	}
	hostname = strings.ToLower(hostnameBadRe.ReplaceAllString(hostname, "-"))
	if !strings.Contains(hostname, ".") {
		hostname += ".local" // VirtualBox requires a fully qualified name
	}
	args := []string{"unattended", "install", vm, "--iso=" + iso, "--user=" + user,
		"--password-file=" + pwFile, "--full-user-name=" + fullName, "--hostname=" + hostname}
	if additions {
		args = append(args, "--install-additions")
	}
	if locale != "" {
		args = append(args, "--locale="+locale)
	}
	if tz != "" {
		args = append(args, "--time-zone="+tz)
	}
	if imageIndex > 0 {
		args = append(args, "--image-index="+strconv.Itoa(imageIndex))
	}
	if productKey != "" {
		args = append(args, "--key="+productKey)
	}
	if startVM == "" {
		startVM = "headless"
	}
	return append(args, "--start-vm="+startVM)
}

func detectedSummary(det map[string]string) map[string]string {
	keep := map[string]string{}
	for k, v := range det {
		if strings.HasPrefix(k, "OS") || strings.HasPrefix(k, "Image") || k == "warning" || k == "IsInstallSupported" {
			keep[k] = v
		}
	}
	return keep
}

func (d *deps) vmState(ctx context.Context, vm string) (string, error) {
	out, err := d.vb.Run(ctx, "showvminfo", vm, "--machinereadable")
	if err != nil {
		return "", err
	}
	return vbox.ParseMachineReadable(out)["VMState"], nil
}

type attachedDisk struct{ slot, path, uuid string }

var hostnameBadRe = regexp.MustCompile(`[^A-Za-z0-9.-]+`)

var slotRe = regexp.MustCompile(`^(.+)-(\d+)-(\d+)$`)

// hardDisks lists attached hard disks from showvminfo: "SATA-0-0"="C:\..\x.vdi" with a
// matching "SATA-ImageUUID-0-0". DVD/floppy images are skipped by extension.
func hardDisks(info map[string]string) []attachedDisk {
	var disks []attachedDisk
	for k, v := range info {
		m := slotRe.FindStringSubmatch(k)
		if m == nil || strings.Contains(k, "ImageUUID") || v == "none" || v == "emptydrive" {
			continue
		}
		low := strings.ToLower(v)
		if !(strings.HasSuffix(low, ".vdi") || strings.HasSuffix(low, ".vmdk") || strings.HasSuffix(low, ".vhd") || strings.HasSuffix(low, ".vhdx")) {
			continue
		}
		uuid := info[m[1]+"-ImageUUID-"+m[2]+"-"+m[3]]
		if uuid == "" {
			continue
		}
		disks = append(disks, attachedDisk{slot: k, path: v, uuid: uuid})
	}
	sort.Slice(disks, func(i, j int) bool { return disks[i].slot < disks[j].slot })
	return disks
}

func pickDisk(info map[string]string, want string) (attachedDisk, bool) {
	disks := hardDisks(info)
	if want == "" && len(disks) > 0 {
		return disks[0], true
	}
	for _, dk := range disks {
		if strings.EqualFold(want, dk.slot) || strings.EqualFold(want, dk.uuid) || strings.EqualFold(want, dk.path) {
			return dk, true
		}
	}
	return attachedDisk{}, false
}

func diskSlots(info map[string]string) []string {
	var s []string
	for _, dk := range hardDisks(info) {
		s = append(s, dk.slot+"="+dk.path)
	}
	if len(s) == 0 {
		return []string{"(none)"}
	}
	return s
}

var capacityRe = regexp.MustCompile(`(?m)^Capacity:\s+(\d+)\s*MBytes`)

func mediumCapacityMB(showmediuminfo string) int {
	if m := capacityRe.FindStringSubmatch(showmediuminfo); m != nil {
		n, _ := strconv.Atoi(m[1])
		return n
	}
	return 0
}
