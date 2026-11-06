package tools

import (
	"context"
	"fmt"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func registerLifecycleExt(s *mcp.Server, d *deps) {
	add(s, "vm_clone",
		"Clone a VM. A full clone is independent; a linked clone is fast and disk-cheap but depends on a "+
			"snapshot of the source VM (pass 'snapshot', and the source keeps that snapshot). The source VM "+
			"should be powered off.",
		object([]string{"vm", "name"},
			p("vm", str("Source VM name or UUID")),
			p("name", str("Name for the new (cloned) VM")),
			p("mode", def(enum("What to clone: 'machine' (current state only), 'all' (all snapshots)", "machine", "all"), "machine")),
			p("linked", def(boolean("Create a linked clone (fast, requires a snapshot) instead of a full copy"), false)),
			p("snapshot", str("Snapshot name to clone/link from (required for linked clones)")),
			p("basedir", str("Base directory for the new VM's files")),
		),
		func(ctx context.Context, in struct {
			VM       string `json:"vm"`
			Name     string `json:"name"`
			Mode     string `json:"mode"`
			Linked   bool   `json:"linked"`
			Snapshot string `json:"snapshot"`
			Basedir  string `json:"basedir"`
		}) (*mcp.CallToolResult, error) {
			if in.Linked && in.Snapshot == "" {
				return errText("A linked clone requires a snapshot to link from. Take one with vm_snapshot(action='take') and pass its name as 'snapshot'."), nil
			}
			args := []string{"clonevm", in.VM, "--name", in.Name, "--register"}
			if in.Mode == "all" {
				args = append(args, "--mode", "all")
			}
			if in.Snapshot != "" {
				args = append(args, "--snapshot", in.Snapshot)
			}
			if in.Linked {
				args = append(args, "--options", "link")
			}
			if in.Basedir != "" {
				args = append(args, "--basefolder", in.Basedir)
			}
			if _, err := d.vb.Run(ctx, args...); err != nil {
				return fail(err), nil
			}
			kind := "full"
			if in.Linked {
				kind = "linked"
			}
			return text(fmt.Sprintf("Cloned %q -> %q (%s clone).", in.VM, in.Name, kind)), nil
		})

	add(s, "vm_export",
		"Export a VM to an OVA/OVF appliance file for backup or sharing. The VM should be powered off.",
		object([]string{"vm", "output"},
			p("vm", str("VM name or UUID")),
			p("output", str("Output path, e.g. /path/to/appliance.ova (.ova = single file, .ovf = directory of files)")),
		),
		func(ctx context.Context, in struct {
			VM     string `json:"vm"`
			Output string `json:"output"`
		}) (*mcp.CallToolResult, error) {
			if _, err := d.vb.Run(ctx, "export", in.VM, "--output", in.Output); err != nil {
				return fail(err), nil
			}
			return text(fmt.Sprintf("Exported %q -> %q.", in.VM, in.Output)), nil
		})

	add(s, "vm_import",
		"Import a VM from an OVA/OVF appliance file, registering it with VirtualBox.",
		object([]string{"file"},
			p("file", str("Path to the .ova or .ovf appliance to import")),
			p("name", str("Optional new name for the imported VM (default: name stored in the appliance)")),
		),
		func(ctx context.Context, in struct {
			File string `json:"file"`
			Name string `json:"name"`
		}) (*mcp.CallToolResult, error) {
			args := []string{"import", in.File}
			if in.Name != "" {
				args = append(args, "--vsys", "0", "--vmname", in.Name)
			}
			if _, err := d.vb.Run(ctx, args...); err != nil {
				return fail(err), nil
			}
			return text(fmt.Sprintf("Imported %q.", in.File)), nil
		})

	add(s, "vm_rename",
		"Rename a VM. The VM must be powered off.",
		object([]string{"vm", "newName"},
			p("vm", str("Current VM name or UUID")),
			p("newName", str("New name for the VM")),
		),
		func(ctx context.Context, in struct {
			VM      string `json:"vm"`
			NewName string `json:"newName"`
		}) (*mcp.CallToolResult, error) {
			if _, err := d.vb.Run(ctx, "modifyvm", in.VM, "--name", in.NewName); err != nil {
				return fail(err), nil
			}
			return text(fmt.Sprintf("Renamed %q -> %q.", in.VM, in.NewName)), nil
		})
}
