package tools

import (
	"context"
	"fmt"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/Rishaad-AWN/mcp-virtualbox/internal/vbox"
)

// legacyOSTypes lack SATA/AHCI drivers and need an IDE controller for the disk.
var legacyOSTypes = map[string]bool{
	"WindowsXP": true, "WindowsXP_64": true,
	"Windows2000": true, "Windows2003": true, "Windows2003_64": true,
	"WindowsMe": true, "Windows98": true, "Windows95": true,
	"WindowsNT4": true, "WindowsNT3x": true,
	"Windows31": true,
	"DOS":       true,
}

var legacyOSRe = regexp.MustCompile(`(?i)^(WindowsXP|Windows200[03]|Windows9|WindowsMe|WindowsNT|Windows31|DOS)`)

func isLegacyOS(osTypeID string) bool {
	return legacyOSTypes[osTypeID] || legacyOSRe.MatchString(osTypeID)
}

type osDefaults struct{ memoryMB, cpus, diskSizeMB int }

func getOSDefaults(osTypeID string) osDefaults {
	match := func(pat string) bool { return regexp.MustCompile(`(?i)^(` + pat + `)`).MatchString(osTypeID) }
	switch {
	case match(`DOS|Windows31|Windows95`):
		return osDefaults{64, 1, 2000}
	case match(`Windows98|WindowsMe`):
		return osDefaults{256, 1, 4000}
	case match(`WindowsNT|Windows2000`):
		return osDefaults{512, 1, 8000}
	case match(`WindowsXP`):
		return osDefaults{512, 1, 10000}
	case match(`Windows2003`):
		return osDefaults{1024, 1, 20000}
	case match(`WindowsVista|Windows2008|Windows7`):
		return osDefaults{2048, 2, 40000}
	case match(`Windows8|Windows2012|Windows10|Windows11|Windows2016|Windows2019|Windows2022`):
		return osDefaults{4096, 2, 50000}
	case match(`Ubuntu|Debian|Fedora|RedHat|Linux`):
		return osDefaults{2048, 2, 25000}
	default:
		return osDefaults{2048, 2, 40000}
	}
}

func registerLifecycle(s *mcp.Server, d *deps) {
	add(s, "vm_list", "List all VirtualBox VMs with their state", object(nil),
		func(ctx context.Context, _ struct{}) (*mcp.CallToolResult, error) {
			return d.listVMs(ctx)
		})

	add(s, "vm_info", "Get detailed VM info",
		object([]string{"vm"}, p("vm", str("VM name or UUID"))),
		func(ctx context.Context, in struct {
			VM string `json:"vm"`
		}) (*mcp.CallToolResult, error) {
			out, err := d.vb.Run(ctx, "showvminfo", in.VM, "--machinereadable")
			if err != nil {
				return fail(err), nil
			}
			info := vbox.ParseMachineReadable(out)
			if isInaccessible(info["name"]) {
				return errText(d.inaccessibleReason(ctx, in.VM)), nil
			}
			return jsonText(info), nil
		})

	add(s, "vm_create",
		"Create a new VM with OS type, RAM, disk, CPU, controllers, and USB tablet mouse. "+
			"Auto-detects legacy OS types (WindowsXP, Windows2000, DOS, etc.) and uses IDE instead of SATA "+
			"since they lack AHCI drivers. Defaults for RAM/CPU/disk are chosen based on OS type if not specified.",
		object([]string{"name", "osTypeId"},
			p("name", str("VM name")),
			p("osTypeId", str("OS type ID (use vm_list_ostypes to find)")),
			p("memoryMB", integer("RAM in MB (auto-detected from OS type if not set)")),
			p("cpus", integer("Number of CPUs (auto-detected from OS type if not set)")),
			p("diskSizeMB", integer("Disk size in MB (auto-detected from OS type if not set)")),
			p("basedir", str("Base directory for VM files")),
		),
		func(ctx context.Context, in struct {
			Name       string `json:"name"`
			OSTypeID   string `json:"osTypeId"`
			MemoryMB   int    `json:"memoryMB"`
			CPUs       int    `json:"cpus"`
			DiskSizeMB int    `json:"diskSizeMB"`
			Basedir    string `json:"basedir"`
		}) (*mcp.CallToolResult, error) {
			return d.createVM(ctx, in.Name, in.OSTypeID, in.MemoryMB, in.CPUs, in.DiskSizeMB, in.Basedir)
		})

	add(s, "vm_start", "Start a VM",
		object([]string{"vm"},
			p("vm", str("VM name or UUID")),
			p("headless", def(boolean("Start headless (true) or with GUI (false)"), true)),
		),
		func(ctx context.Context, in struct {
			VM       string `json:"vm"`
			Headless bool   `json:"headless"`
		}) (*mcp.CallToolResult, error) {
			typ := "gui"
			if in.Headless {
				typ = "headless"
			}
			if _, err := d.vb.Run(ctx, "startvm", in.VM, "--type", typ); err != nil {
				return fail(err), nil
			}
			return text(fmt.Sprintf("VM %q started (%s).", in.VM, typ)), nil
		})

	add(s, "vm_stop",
		"Stop a VM. Default 'auto' presses the ACPI power button (clean guest shutdown), waits up to "+
			"timeoutSec, then forces power off only if the guest ignored it. 'poweroff' cuts power at once "+
			"(like pulling the plug: unsaved guest data can be lost); 'acpipowerbutton' only asks and returns; "+
			"'savestate' hibernates the VM to disk.",
		object([]string{"vm"},
			p("vm", str("VM name or UUID")),
			p("method", def(enum("Stop method", "auto", "poweroff", "acpipowerbutton", "savestate"), "auto")),
			p("timeoutSec", def(integer("auto: seconds to wait for the clean shutdown before forcing power off"), 60)),
		),
		func(ctx context.Context, in struct {
			VM         string `json:"vm"`
			Method     string `json:"method"`
			TimeoutSec int    `json:"timeoutSec"`
		}) (*mcp.CallToolResult, error) {
			if in.Method != "auto" {
				if _, err := d.vb.Run(ctx, "controlvm", in.VM, in.Method); err != nil {
					return fail(err), nil
				}
				if in.Method == "acpipowerbutton" {
					return text(fmt.Sprintf("ACPI power button pressed on %q; the guest is shutting down "+
						"(use vm_wait_for_state state=poweroff to wait for it).", in.VM)), nil
				}
				return text(fmt.Sprintf("VM %q stopped (%s).", in.VM, in.Method)), nil
			}
			st, err := d.vmState(ctx, in.VM)
			if err != nil {
				return fail(err), nil
			}
			switch st {
			case "poweroff", "aborted", "saved", "":
				return text(fmt.Sprintf("VM %q is not running (state: %s); nothing to stop.", in.VM, st)), nil
			case "running":
			default:
				// paused, stuck (stopping/starting) or crashed (gurumeditation): ACPI can't help
				if _, err := d.vb.Run(ctx, "controlvm", in.VM, "poweroff"); err != nil {
					return fail(err), nil
				}
				return text(fmt.Sprintf("VM %q was %s, so it was powered off directly.", in.VM, st)), nil
			}
			timeout := in.TimeoutSec
			if timeout <= 0 {
				timeout = 60
			}
			start := time.Now()
			why := fmt.Sprintf("did not shut down within %ds of the ACPI request (no ACPI support, or the guest ignored it)", timeout)
			if _, err := d.vb.Run(ctx, "controlvm", in.VM, "acpipowerbutton"); err != nil {
				why = "could not receive the ACPI request (" + vbox.FormatError(err) + ")"
			} else {
				get := func() (string, error) { return d.vmState(ctx, in.VM) }
				if _, ok, err := waitFor(ctx, time.Duration(timeout)*time.Second, time.Second, get, "poweroff"); err != nil {
					return fail(err), nil
				} else if ok {
					return text(fmt.Sprintf("VM %q shut down cleanly (ACPI) in %ds.", in.VM, int(time.Since(start).Seconds()))), nil
				}
			}
			if _, err := d.vb.Run(ctx, "controlvm", in.VM, "poweroff"); err != nil {
				return fail(err), nil
			}
			return text(fmt.Sprintf("VM %q %s, so it was forced off.", in.VM, why)), nil
		})

	add(s, "vm_delete", "Remove a VM from VirtualBox. By default only unregisters it (files kept); deleteFiles=true also erases its disks. Also the way to clean up an inaccessible VM.",
		object([]string{"vm"},
			p("vm", str("VM name or UUID")),
			p("deleteFiles", def(boolean("Also permanently delete all its files (disk images, snapshots). "+
				"Default false: only unregister, so the VM can be re-added later. Set true only when the user "+
				"explicitly asked to delete the disks."), false)),
		),
		func(ctx context.Context, in struct {
			VM          string `json:"vm"`
			DeleteFiles bool   `json:"deleteFiles"`
		}) (*mcp.CallToolResult, error) {
			args := []string{"unregistervm", in.VM}
			if in.DeleteFiles {
				args = append(args, "--delete")
			}
			if _, err := d.vb.Run(ctx, args...); err != nil {
				return fail(err), nil
			}
			suffix := " (unregistered only)"
			if in.DeleteFiles {
				suffix = " (with all files)"
			}
			return text(fmt.Sprintf("VM %q deleted%s.", in.VM, suffix)), nil
		})

	addControl(s, d, "vm_pause", "Pause a running VM", "pause", "paused")
	addControl(s, d, "vm_resume", "Resume a paused VM", "resume", "resumed")
	addControl(s, d, "vm_reset", "Hard reset a running VM (like pressing the reset button)", "reset", "reset")

	add(s, "vm_shared_folder_add",
		"Add a shared folder between host and guest. Requires Guest Additions in the guest. "+
			"The folder appears as a network drive in Windows guests or can be mounted in Linux guests.",
		object([]string{"vm", "name", "hostPath"},
			p("vm", str("VM name or UUID")),
			p("name", str("Share name (used to identify the folder in the guest)")),
			p("hostPath", str("Absolute path on the host to share")),
			p("writable", def(boolean("Allow the guest to write to the folder"), true)),
			p("automount", def(boolean("Auto-mount in the guest"), true)),
			p("transient", def(boolean("Temporary share that does not persist across VM restarts (can be added while VM is running)"), false)),
		),
		func(ctx context.Context, in struct {
			VM        string `json:"vm"`
			Name      string `json:"name"`
			HostPath  string `json:"hostPath"`
			Writable  bool   `json:"writable"`
			Automount bool   `json:"automount"`
			Transient bool   `json:"transient"`
		}) (*mcp.CallToolResult, error) {
			args := []string{"sharedfolder", "add", in.VM, "--name", in.Name, "--hostpath", in.HostPath}
			if in.Transient {
				args = append(args, "--transient")
			}
			if !in.Writable {
				args = append(args, "--readonly")
			}
			if in.Automount {
				args = append(args, "--automount")
			}
			if _, err := d.vb.Run(ctx, args...); err != nil {
				return fail(err), nil
			}
			mode := "read-write"
			if !in.Writable {
				mode = "read-only"
			}
			extra := ""
			if in.Transient {
				extra = ", transient"
			}
			return text(fmt.Sprintf("Shared folder %q added: host %q -> guest (%s%s).", in.Name, in.HostPath, mode, extra)), nil
		})

	add(s, "vm_shared_folder_remove", "Remove a shared folder from a VM",
		object([]string{"vm", "name"},
			p("vm", str("VM name or UUID")),
			p("name", str("Share name to remove")),
			p("transient", def(boolean("Remove a transient share (must match how it was added)"), false)),
		),
		func(ctx context.Context, in struct {
			VM        string `json:"vm"`
			Name      string `json:"name"`
			Transient bool   `json:"transient"`
		}) (*mcp.CallToolResult, error) {
			args := []string{"sharedfolder", "remove", in.VM, "--name", in.Name}
			if in.Transient {
				args = append(args, "--transient")
			}
			if _, err := d.vb.Run(ctx, args...); err != nil {
				return fail(err), nil
			}
			return text(fmt.Sprintf("Shared folder %q removed from %q.", in.Name, in.VM)), nil
		})

	add(s, "vm_attach_media",
		"Attach ISO/disk to VM, eject media, or attach Guest Additions ISO. For legacy OS VMs (XP, 2000, etc.) "+
			"ISO goes on IDE port 1 device 0. For modern OS VMs, ISO goes on IDE port 0 device 0.",
		object([]string{"vm", "medium"},
			p("vm", str("VM name or UUID")),
			p("storagectl", def(str("Storage controller name"), "IDE")),
			p("port", def(integer("Port number"), 0)),
			p("device", def(integer("Device number"), 0)),
			p("type", def(enum("Medium type", "dvddrive", "hdd"), "dvddrive")),
			p("medium", str("Path to ISO/disk, 'emptydrive' to eject, or 'additions' for Guest Additions ISO")),
		),
		func(ctx context.Context, in struct {
			VM         string `json:"vm"`
			Storagectl string `json:"storagectl"`
			Port       int    `json:"port"`
			Device     int    `json:"device"`
			Type       string `json:"type"`
			Medium     string `json:"medium"`
		}) (*mcp.CallToolResult, error) {
			if _, err := d.vb.Run(ctx, "storageattach", in.VM,
				"--storagectl", in.Storagectl, "--port", strconv.Itoa(in.Port),
				"--device", strconv.Itoa(in.Device), "--type", in.Type, "--medium", in.Medium); err != nil {
				return fail(err), nil
			}
			return text(fmt.Sprintf("Attached %q to %s (%s port %d).", in.Medium, in.VM, in.Storagectl, in.Port)), nil
		})

	add(s, "vm_snapshot", "Manage VM snapshots",
		object([]string{"vm", "action"},
			p("vm", str("VM name or UUID")),
			p("action", enum("Snapshot action", "take", "restore", "delete", "list")),
			p("snapshotName", str("Snapshot name (required for take/restore/delete)")),
		),
		func(ctx context.Context, in struct {
			VM           string `json:"vm"`
			Action       string `json:"action"`
			SnapshotName string `json:"snapshotName"`
		}) (*mcp.CallToolResult, error) {
			if in.Action == "list" {
				out, err := d.vb.Run(ctx, "snapshot", in.VM, "list", "--machinereadable")
				if err != nil {
					if strings.Contains(vbox.FormatError(err), "does not have any snapshots") {
						return text("No snapshots."), nil
					}
					return errText("Snapshot list failed: " + vbox.FormatError(err)), nil
				}
				return text(out), nil
			}
			if in.SnapshotName == "" {
				return errText("snapshotName is required for take/restore/delete."), nil
			}
			if in.Action == "restore" {
				if st, err := d.vmState(ctx, in.VM); err == nil && st != "poweroff" && st != "aborted" && st != "saved" {
					return errText(fmt.Sprintf("Cannot restore a snapshot while VM %q is %s: stop it first "+
						"(vm_stop), then restore.", in.VM, st)), nil
				}
			}
			if _, err := d.vb.Run(ctx, "snapshot", in.VM, in.Action, in.SnapshotName); err != nil {
				return fail(err), nil
			}
			return text(fmt.Sprintf("Snapshot %q %s completed on %q.", in.SnapshotName, in.Action, in.VM)), nil
		})

	add(s, "vm_modify", "Modify arbitrary VM settings via VBoxManage modifyvm",
		object([]string{"vm", "args"},
			p("vm", str("VM name or UUID")),
			p("args", array("Arguments to pass to modifyvm (e.g. ['--memory', '4096'])", str(""))),
		),
		func(ctx context.Context, in struct {
			VM   string   `json:"vm"`
			Args []string `json:"args"`
		}) (*mcp.CallToolResult, error) {
			if _, err := d.vb.Run(ctx, append([]string{"modifyvm", in.VM}, in.Args...)...); err != nil {
				return fail(err), nil
			}
			return text(fmt.Sprintf("Modified %q with args: %s", in.VM, strings.Join(in.Args, " "))), nil
		})

	add(s, "vm_list_ostypes", "List available OS type IDs for VM creation", object(nil),
		func(ctx context.Context, _ struct{}) (*mcp.CallToolResult, error) {
			out, err := d.vb.Run(ctx, "list", "ostypes")
			if err != nil {
				return fail(err), nil
			}
			return jsonText(parseOSTypes(out)), nil
		})
}

// addControl registers a one-arg controlvm verb tool (pause/resume/reset).
func addControl(s *mcp.Server, d *deps, name, desc, verb, past string) {
	add(s, name, desc, object([]string{"vm"}, p("vm", str("VM name or UUID"))),
		func(ctx context.Context, in struct {
			VM string `json:"vm"`
		}) (*mcp.CallToolResult, error) {
			if _, err := d.vb.Run(ctx, "controlvm", in.VM, verb); err != nil {
				return fail(err), nil
			}
			return text(fmt.Sprintf("VM %q %s.", in.VM, past)), nil
		})
}

// vmWithState augments a VM with its current run state.
type vmWithState struct {
	Name  string `json:"name"`
	UUID  string `json:"uuid"`
	State string `json:"state"`
}

// listVMs returns every VM with its precise state (running, paused, saved,
// aborted, poweroff), querying showvminfo per VM concurrently.
func (d *deps) listVMs(ctx context.Context) (*mcp.CallToolResult, error) {
	allOut, err := d.vb.Run(ctx, "list", "vms")
	if err != nil {
		return fail(err), nil
	}
	vms := vbox.ParseVMList(allOut)

	// Fallback heuristic set, in case a per-VM query fails.
	runningOut, _ := d.vb.Run(ctx, "list", "runningvms")
	running := make(map[string]bool)
	for _, v := range vbox.ParseVMList(runningOut) {
		running[v.UUID] = true
	}

	result := make([]vmWithState, len(vms))
	sem := make(chan struct{}, 8)
	var wg sync.WaitGroup
	for i, vm := range vms {
		wg.Add(1)
		sem <- struct{}{}
		go func(i int, vm vbox.VM) {
			defer wg.Done()
			defer func() { <-sem }()
			state := "poweroff"
			if running[vm.UUID] {
				state = "running"
			}
			if isInaccessible(vm.Name) {
				state = "inaccessible" // settings file missing/unreadable; showvminfo has no VMState for it
			} else if out, err := d.vb.Run(ctx, "showvminfo", vm.UUID, "--machinereadable"); err == nil {
				if vs := vbox.ParseMachineReadable(out)["VMState"]; vs != "" {
					state = vs
				}
			}
			result[i] = vmWithState{Name: vm.Name, UUID: vm.UUID, State: state}
		}(i, vm)
	}
	wg.Wait()
	return jsonText(result), nil
}

// createVM creates and configures a VM, rolling back a half-created VM if a
// later step fails, so no partial VM is left registered.
func (d *deps) createVM(ctx context.Context, name, osTypeID string, memoryMB, cpus, diskSizeMB int, basedir string) (*mcp.CallToolResult, error) {
	defaults := getOSDefaults(osTypeID)
	if memoryMB <= 0 {
		memoryMB = defaults.memoryMB
	}
	if cpus <= 0 {
		cpus = defaults.cpus
	}
	if diskSizeMB <= 0 {
		diskSizeMB = defaults.diskSizeMB
	}
	legacy := isLegacyOS(osTypeID)

	createArgs := []string{"createvm", "--name", name, "--ostype", osTypeID, "--register"}
	if basedir != "" {
		createArgs = append(createArgs, "--basefolder", basedir)
	}
	if _, err := d.vb.Run(ctx, createArgs...); err != nil {
		return fail(err), nil
	}

	// Roll back the freshly-registered VM if anything below fails.
	created := true
	rollback := func() {
		if created {
			_, _ = d.vb.Run(ctx, "unregistervm", name, "--delete")
		}
	}
	ok := func() { created = false }

	infoOut, err := d.vb.Run(ctx, "showvminfo", name, "--machinereadable")
	if err != nil {
		rollback()
		return fail(err), nil
	}
	cfgFile := vbox.ParseMachineReadable(infoOut)["CfgFile"]
	vmDir := filepath.Dir(cfgFile)

	if _, err := d.vb.Run(ctx, "modifyvm", name,
		"--memory", strconv.Itoa(memoryMB),
		"--cpus", strconv.Itoa(cpus),
		"--mouse", "usbtablet",
		"--graphicscontroller", "vboxsvga",
		"--vram", "128",
		"--audio-enabled", "off",
		"--nic1", "nat",
	); err != nil {
		rollback()
		return fail(err), nil
	}

	diskPath := filepath.Join(vmDir, name+".vdi")
	if _, err := d.vb.Run(ctx, "createmedium", "disk", "--filename", diskPath,
		"--size", strconv.Itoa(diskSizeMB), "--format", "VDI"); err != nil {
		rollback()
		return fail(err), nil
	}

	if legacy {
		if err := runAll(ctx, d.vb,
			[]string{"storagectl", name, "--name", "IDE", "--add", "ide"},
			[]string{"storageattach", name, "--storagectl", "IDE", "--port", "0", "--device", "0", "--type", "hdd", "--medium", diskPath},
		); err != nil {
			rollback()
			return fail(err), nil
		}
	} else {
		if err := runAll(ctx, d.vb,
			[]string{"storagectl", name, "--name", "SATA", "--add", "sata", "--controller", "IntelAhci", "--portcount", "2"},
			[]string{"storagectl", name, "--name", "IDE", "--add", "ide"},
			[]string{"storageattach", name, "--storagectl", "SATA", "--port", "0", "--device", "0", "--type", "hdd", "--medium", diskPath},
		); err != nil {
			rollback()
			return fail(err), nil
		}
	}
	ok()

	storageNote := "Storage: SATA for HDD, IDE for DVD. Attach ISO to IDE port 0."
	if legacy {
		storageNote = "Storage: IDE only (legacy OS - no AHCI/SATA drivers). HDD on IDE port 0, attach ISO to IDE port 1."
	}
	return text(fmt.Sprintf(
		"VM %q created successfully.\nOS: %s, RAM: %dMB, CPUs: %d, Disk: %dMB\n"+
			"USB tablet mouse enabled for absolute positioning.\n%s",
		name, osTypeID, memoryMB, cpus, diskSizeMB, storageNote)), nil
}

func runAll(ctx context.Context, vb *vbox.Client, cmds ...[]string) error {
	for _, c := range cmds {
		if _, err := vb.Run(ctx, c...); err != nil {
			return err
		}
	}
	return nil
}

type osType struct {
	ID          string `json:"id"`
	Description string `json:"description"`
}

var (
	osCombinedRe = regexp.MustCompile(`^ID\s*/\s*Description:\s+(\S+)\s+--\s+(.+)`)
	osIDRe       = regexp.MustCompile(`^ID:\s+(.+)`)
	osDescRe     = regexp.MustCompile(`^Description:\s+(.+)`)
)

// parseOSTypes handles both the VirtualBox 7.1+ combined format
// ("ID / Description: WindowsXP -- Windows XP") and the older two-line format.
func parseOSTypes(output string) []osType {
	var types []osType
	var currentID string
	for _, line := range strings.Split(output, "\n") {
		if m := osCombinedRe.FindStringSubmatch(line); m != nil {
			types = append(types, osType{ID: strings.TrimSpace(m[1]), Description: strings.TrimSpace(m[2])})
			continue
		}
		if m := osIDRe.FindStringSubmatch(line); m != nil {
			currentID = strings.TrimSpace(m[1])
		}
		if m := osDescRe.FindStringSubmatch(line); m != nil && currentID != "" {
			types = append(types, osType{ID: currentID, Description: strings.TrimSpace(m[1])})
			currentID = ""
		}
	}
	return types
}

func isInaccessible(name string) bool { return strings.HasPrefix(name, "<inaccessible") }

// inaccessibleReason explains a VM whose settings file VirtualBox cannot read.
// The human-readable showvminfo prints the config path; machinereadable does not.
func (d *deps) inaccessibleReason(ctx context.Context, vm string) string {
	cfg := "unknown"
	if out, err := d.vb.Run(ctx, "showvminfo", vm); err == nil {
		for _, line := range strings.Split(out, "\n") {
			if strings.HasPrefix(line, "Config file:") {
				cfg = strings.TrimSpace(strings.TrimPrefix(line, "Config file:"))
			}
		}
	}
	return fmt.Sprintf("VM %s is inaccessible: VirtualBox cannot read its settings file %s "+
		"(moved, deleted, or on a drive that is not connected). Restore that file and restart VirtualBox, "+
		"or remove the dead entry with vm_delete (deleteFiles=false).", vm, cfg)
}

// waitFor polls get every poll until it returns one of want, the timeout passes, or
// ctx ends. It returns the last state seen and whether it matched.
func waitFor(ctx context.Context, timeout, poll time.Duration, get func() (string, error), want ...string) (string, bool, error) {
	deadline := time.Now().Add(timeout)
	for {
		st, err := get()
		if err != nil {
			return "", false, err
		}
		for _, w := range want {
			if st == w {
				return st, true, nil
			}
		}
		if time.Now().Add(poll).After(deadline) {
			return st, false, nil
		}
		select {
		case <-ctx.Done():
			return st, false, ctx.Err()
		case <-time.After(poll):
		}
	}
}

func registerWaitForState(s *mcp.Server, d *deps) {
	add(s, "vm_wait_for_state",
		"Wait until a VM reaches a state (e.g. poweroff after a shutdown, running after start). "+
			"Returns the state reached, or the last state seen on timeout (a VM stuck in 'stopping' shows up here).",
		object([]string{"vm", "state"},
			p("vm", str("VM name or UUID")),
			p("state", enum("State to wait for", "poweroff", "running", "paused", "saved", "aborted")),
			p("timeoutSec", def(integer("Max seconds to wait"), 120)),
		),
		func(ctx context.Context, in struct {
			VM         string `json:"vm"`
			State      string `json:"state"`
			TimeoutSec int    `json:"timeoutSec"`
		}) (*mcp.CallToolResult, error) {
			if in.TimeoutSec <= 0 {
				in.TimeoutSec = 120
			}
			start := time.Now()
			get := func() (string, error) { return d.vmState(ctx, in.VM) }
			st, ok, err := waitFor(ctx, time.Duration(in.TimeoutSec)*time.Second, time.Second, get, in.State)
			if err != nil {
				return fail(err), nil
			}
			if !ok {
				return errText(fmt.Sprintf("Timed out after %ds: VM %q is %q, not %q.", in.TimeoutSec, in.VM, st, in.State)), nil
			}
			return text(fmt.Sprintf("VM %q is %s (after %ds).", in.VM, st, int(time.Since(start).Seconds()))), nil
		})
}
