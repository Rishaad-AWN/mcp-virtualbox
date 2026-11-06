package tools

import (
	"context"
	"fmt"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type recommendedSettings struct {
	memoryMB, cpus, diskSizeMB int
	diskController             string
}

type installGuide struct {
	osType        string
	displayName   string
	settings      recommendedSettings
	installSteps  []string
	gotchas       []string
	postInstall   []string
	guestExecTips []string
}

var windowsGuestExecTips = []string{
	"CRITICAL: Windows accounts with BLANK passwords CANNOT use vm_guest_exec. You MUST set a password first via the vision loop: Win+R -> cmd -> 'net user <username> <password>'",
	`Use executable='C:\WINDOWS\system32\cmd.exe' with args=['/c', 'command', 'arg1', 'arg2']`,
	`Each argument MUST be a SEPARATE array element. WRONG: ['/c', 'dir C:\'] - RIGHT: ['/c', 'dir', 'C:\']`,
	"VBoxManage guestcontrol BREAKS arguments containing spaces (e.g. 'Program Files', 'Windows NT'). The spaces cause argument splitting on the guest side.",
	"Workaround for commands with spaces: use vm_guest_copy_to to write a .bat script to the guest, then execute the .bat with vm_guest_exec",
	"Alternative workaround: use 8.3 short names (e.g. 'PROGRA~1' instead of 'Program Files')",
	"For complex multi-step operations, write a batch script and copy it to the guest rather than running many individual commands",
}

var installGuides = map[string]installGuide{
	"windowsxp": {
		osType:      "WindowsXP",
		displayName: "Windows XP Professional SP3",
		settings:    recommendedSettings{512, 1, 10000, "IDE"},
		installSteps: []string{
			"1. Create VM with osTypeId='WindowsXP' (vm_create auto-uses IDE for disk)",
			"2. Attach ISO to IDE port 1 device 0: vm_attach_media(storagectl='IDE', port=1, device=0, type='dvddrive')",
			"3. Start VM headless, wait 5-10s for boot",
			"4. Screenshot -> 'Welcome to Setup' screen -> press ENTER to install",
			"5. Screenshot -> EULA -> press F8 to agree",
			"6. Screenshot -> Partition screen -> press ENTER to use unpartitioned space",
			"7. Screenshot -> Format screen -> select 'NTFS (Quick)' (arrow up to it) -> ENTER",
			"8. Wait ~2-5 min for file copy + reboot. Take screenshots every 15-20s to monitor",
			"9. After reboot, GUI setup starts automatically. Screens in order:",
			"   a. Regional settings -> press ENTER (Next)",
			"   b. Name/Organization -> type name, press ENTER",
			"   c. Product Key -> enter 25-char VL key (tab auto-advances between fields)",
			"   d. Computer name + admin password -> fill in, ENTER",
			"   e. Date/Time -> press ENTER",
			"   f. Network settings -> 'Typical' is selected, press ENTER",
			"   g. Workgroup -> press ENTER",
			"10. Wait ~5-10 min for 'Installing Windows' phase to complete + reboot",
			"11. After final reboot: Welcome screen -> press ENTER, skip registration, set username",
			"12. Desktop appears - XP is installed!",
		},
		gotchas: []string{
			"CRITICAL: Windows XP has NO SATA/AHCI drivers. Disk MUST be on IDE controller, not SATA. vm_create handles this automatically.",
			"IDE layout: port 0 device 0 = HDD, port 1 device 0 = DVD/ISO",
			"VL (Volume License) ISOs require a product key - cannot be skipped",
			"XP only supports 1 CPU effectively (use cpus=1)",
			"512MB RAM is the sweet spot - more than 512MB gives diminishing returns, less causes slow setup",
			"After format + file copy, VM auto-reboots into GUI setup - no intervention needed during reboot",
			"The setup reboots multiple times - don't assume a reboot means it's done",
			`Guest Additions: after install, attach additions ISO and run D:\VBoxWindowsAdditions.exe from inside the VM`,
			"CRITICAL: Windows accounts with BLANK passwords cannot use VBoxManage guestcontrol. You MUST set a password (e.g. 'net user <username> <password>') before vm_guest_exec will work.",
		},
		postInstall: []string{
			"Install Guest Additions for clipboard, shared folders, and better display",
			"Enable WinRM for text-based control: run in cmd as admin:",
			"  winrm quickconfig -q",
			`  winrm set winrm/config/service/auth @{Basic="true"}`,
			`  winrm set winrm/config/service @{AllowUnencrypted="true"}`,
			"Add NAT port forward: vm_add_nat_port_forward(name='winrm', hostPort=5985, guestPort=5985)",
			"Then use vm_exec_winrm for efficient text-based control",
		},
		guestExecTips: windowsGuestExecTips,
	},
	"windows7": {
		osType:      "Windows7_64",
		displayName: "Windows 7",
		settings:    recommendedSettings{2048, 2, 40000, "SATA"},
		installSteps: []string{
			"1. Create VM with osTypeId='Windows7_64'",
			"2. Attach ISO to IDE port 0: vm_attach_media(storagectl='IDE', port=0, device=0, type='dvddrive')",
			"3. Start VM, wait for 'Press any key to boot from CD/DVD' -> send ENTER quickly",
			"4. Language selection -> ENTER for defaults",
			"5. 'Install now' button -> click or press ENTER",
			"6. License terms -> check 'I accept' (click checkbox) -> Next",
			"7. Installation type -> 'Custom (advanced)'",
			"8. Disk selection -> select unallocated space -> Next",
			"9. Wait 15-30 min for install + reboots",
			"10. After final reboot: username, password, product key, updates, time zone",
			"11. Desktop appears - Windows 7 installed!",
		},
		gotchas: []string{
			"Windows 7 supports SATA/AHCI - use default SATA controller",
			"'Press any key to boot from CD' timeout is short - send a key quickly after start",
			"After first reboot, do NOT press a key to boot from CD again - let it boot from disk",
			"VBoxSVGA graphics controller works well with Windows 7",
			"Windows accounts with BLANK passwords cannot use VBoxManage guestcontrol - set a password first.",
		},
		postInstall: []string{
			"Install Guest Additions for better display and integration",
			"Enable WinRM or install OpenSSH for text-based control",
			"Add NAT port forward for remote access",
		},
		guestExecTips: windowsGuestExecTips,
	},
	"windows10": {
		osType:      "Windows10_64",
		displayName: "Windows 10/11",
		settings:    recommendedSettings{4096, 2, 50000, "SATA"},
		installSteps: []string{
			"1. Create VM with osTypeId='Windows10_64' or 'Windows11_64'",
			"2. For Windows 11: need TPM and Secure Boot - may require registry hack during install",
			"3. Attach ISO to IDE port 0: vm_attach_media(storagectl='IDE', port=0, device=0, type='dvddrive')",
			"4. Start VM, send key at 'Press any key to boot from CD/DVD'",
			"5. Language -> Next -> 'Install now'",
			"6. Product key -> 'I don't have a product key' to skip",
			"7. Select edition -> Next -> Accept license -> Custom install",
			"8. Disk -> select unallocated -> Next",
			"9. Wait 15-30 min for install + reboots",
			"10. OOBE: region, keyboard, network (skip internet if possible), user account",
			"11. Desktop appears!",
		},
		gotchas: []string{
			"Windows 10/11 supports SATA - use default controller",
			"Windows 11 requires TPM 2.0 - VirtualBox 7+ can emulate it",
			`To bypass Win11 TPM check: at install screen, Shift+F10 -> regedit -> add DWORD LabConfig\BypassTPMCheck=1 under HKLM\SYSTEM\Setup`,
			`OOBE tries to force Microsoft account - disconnect network or use bypass (oobe\bypassnro)`,
			"4GB RAM minimum for smooth experience",
			"Windows accounts with BLANK passwords cannot use VBoxManage guestcontrol - set a password first.",
		},
		postInstall: []string{
			"Install Guest Additions",
			"OpenSSH server available in Settings -> Apps -> Optional Features",
			"WinRM: run 'winrm quickconfig -q' in admin PowerShell",
			"Add NAT port forwards for SSH (22) or WinRM (5985)",
		},
		guestExecTips: windowsGuestExecTips,
	},
	"ubuntu": {
		osType:      "Ubuntu_64",
		displayName: "Ubuntu Linux",
		settings:    recommendedSettings{2048, 2, 25000, "SATA"},
		installSteps: []string{
			"1. Create VM with osTypeId='Ubuntu_64'",
			"2. Attach ISO to IDE port 0: vm_attach_media(storagectl='IDE', port=0, device=0, type='dvddrive')",
			"3. Start VM, wait for GRUB menu -> 'Install Ubuntu' or 'Try Ubuntu'",
			"4. Follow installer: language, keyboard, installation type, disk, timezone, user",
			"5. Wait 10-20 min for install",
			"6. Reboot, remove ISO when prompted",
			"7. Login -> desktop ready",
		},
		gotchas: []string{
			"Ubuntu Server: text-mode installer, much faster - good for headless VMs",
			"Ubuntu Desktop installer can be slow in VM - be patient with screenshots",
			"SATA works fine for all modern Linux",
			"SSH server: Ubuntu Server includes it by default; Desktop needs 'sudo apt install openssh-server'",
		},
		postInstall: []string{
			"Install Guest Additions: sudo apt install virtualbox-guest-additions-iso (or mount additions ISO)",
			"SSH is the best way to control Linux VMs - add NAT port forward: vm_add_nat_port_forward(name='ssh', hostPort=2222, guestPort=22)",
			"Then use vm_exec_ssh(port=2222, username='user', password='pass', command='...')",
		},
	},
}

// guideAliases maps user-friendly names to canonical guide keys.
var guideAliases = map[string]string{
	"winxp":     "windowsxp",
	"xp":        "windowsxp",
	"win7":      "windows7",
	"win10":     "windows10",
	"win11":     "windows10",
	"windows11": "windows10",
	"linux":     "ubuntu",
}

func lookupGuide(os string) (installGuide, bool) {
	key := strings.ReplaceAll(strings.ToLower(os), " ", "")
	if canonical, ok := guideAliases[key]; ok {
		key = canonical
	}
	g, ok := installGuides[key]
	return g, ok
}

func registerKnowledge(s *mcp.Server, _ *deps) {
	add(s, "vm_install_guide",
		"Get OS-specific installation guide with step-by-step instructions, gotchas, and recommended "+
			"settings. Call this BEFORE starting an OS installation to avoid common pitfalls. Available "+
			"guides: windowsxp, windows7, windows10, windows11, ubuntu, linux",
		object([]string{"os"},
			p("os", str("OS name: 'windowsxp', 'windows7', 'windows10', 'windows11', 'ubuntu', 'linux'")),
		),
		func(_ context.Context, in struct {
			OS string `json:"os"`
		}) (*mcp.CallToolResult, error) {
			guide, ok := lookupGuide(in.OS)
			if !ok {
				return text(noGuideText(in.OS)), nil
			}
			return text(renderGuide(guide)), nil
		})
}

func noGuideText(os string) string {
	seen := map[string]bool{}
	var names []string
	for _, g := range installGuides {
		if !seen[g.displayName] {
			seen[g.displayName] = true
			names = append(names, g.displayName)
		}
	}
	return fmt.Sprintf(`No guide found for %q. Available guides: %s

General tips:
- Use vm_list_ostypes to find the correct OS type ID
- Legacy OS (XP, 2000, 98, DOS) need IDE controller - vm_create handles this automatically
- Modern OS (Vista+, Linux) work fine with SATA
- Always enable USB tablet mouse (vm_create does this) for accurate mouse clicks
- After install: set up Guest Additions, then SSH/WinRM for text-based control`,
		os, strings.Join(names, ", "))
}

func renderGuide(g installGuide) string {
	var b strings.Builder
	fmt.Fprintf(&b, "# %s Installation Guide\n\n", g.displayName)
	b.WriteString("## Recommended Settings\n")
	fmt.Fprintf(&b, "- OS Type ID: %s\n", g.osType)
	fmt.Fprintf(&b, "- RAM: %dMB\n", g.settings.memoryMB)
	fmt.Fprintf(&b, "- CPUs: %d\n", g.settings.cpus)
	fmt.Fprintf(&b, "- Disk: %dMB\n", g.settings.diskSizeMB)
	fmt.Fprintf(&b, "- Disk Controller: %s\n\n", g.settings.diskController)

	b.WriteString("## Installation Steps\n")
	b.WriteString(strings.Join(g.installSteps, "\n"))
	b.WriteString("\n\n## Gotchas & Common Pitfalls\n")
	for _, x := range g.gotchas {
		fmt.Fprintf(&b, "- %s\n", x)
	}
	b.WriteString("\n## Post-Install Setup\n")
	for _, x := range g.postInstall {
		fmt.Fprintf(&b, "- %s\n", x)
	}
	if len(g.guestExecTips) > 0 {
		b.WriteString("\n## vm_guest_exec Usage (IMPORTANT - read before using)\n")
		for _, x := range g.guestExecTips {
			fmt.Fprintf(&b, "- %s\n", x)
		}
	}
	return strings.TrimRight(b.String(), "\n")
}
