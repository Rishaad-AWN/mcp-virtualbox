package tools

import (
	"strings"
	"testing"

	"github.com/Rishaad-AWN/mcp-virtualbox/internal/hostnet"
)

func TestNormMAC(t *testing.T) {
	for in, want := range map[string]string{
		"08:00:27:8B:3C:9F": "0800278b3c9f",
		"08-00-27-8b-3c-9f": "0800278b3c9f",
		"0800278B3C9F":      "0800278b3c9f",
		"8:0:27:8b:3c:9f":   "0800278b3c9f", // macOS arp drops leading zeros
		"not-a-mac":         "",
		"08:00:27:8b:3c":    "",
	} {
		if got := hostnet.NormMAC(in); got != want {
			t.Errorf("NormMAC(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestParseARPFormats(t *testing.T) {
	out := `
Interface: 192.168.56.1 --- 0x12
  Internet Address      Physical Address      Type
  192.168.56.101        08-00-27-8b-3c-9f     dynamic
  192.168.56.255        ff-ff-ff-ff-ff-ff     static
? (10.0.0.5) at 8:0:27:aa:bb:c on vboxnet0 ifscope [ethernet]
10.0.0.6 dev vboxnet0 lladdr 08:00:27:11:22:33 REACHABLE`
	got := hostnet.ParseARP(out)
	want := map[string]string{"0800278b3c9f": "192.168.56.101", "080027aabb0c": "10.0.0.5", "080027112233": "10.0.0.6"}
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for k, v := range want {
		if got[k] != v {
			t.Errorf("%s -> %q, want %q", k, got[k], v)
		}
	}
}

func TestParseLeases(t *testing.T) {
	data := []byte(`<?xml version="1.0"?>
<Leases version="1.0">
  <Lease mac="08:00:27:8b:3c:9f" id="01" network="0.0.0.0" state="acked"><Address value="192.168.56.101"/><Time issued="1000" expiration="600"/></Lease>
  <Lease mac="08:00:27:00:00:01" id="02" network="0.0.0.0" state="released"><Address value="192.168.56.102"/></Lease>
  <Lease mac="08:00:27:00:00:02" id="03" network="0.0.0.0" state="acked"><Address value="192.168.56.103"/><Time issued="100" expiration="600"/></Lease>
</Leases>`)
	out := map[string]lease{}
	parseLeases(data, 1200, out) // lease 03 expired at 700
	if len(out) != 1 || out["0800278b3c9f"].ip != "192.168.56.101" {
		t.Fatalf("got %v", out)
	}
	// the same MAC on another network with a newer lease wins; an older one does not
	parseLeases([]byte(`<Leases><Lease mac="08:00:27:8b:3c:9f" state="acked"><Address value="10.0.2.15"/><Time issued="1100" expiration="600"/></Lease></Leases>`), 1200, out)
	parseLeases([]byte(`<Leases><Lease mac="08:00:27:8b:3c:9f" state="acked"><Address value="10.9.9.9"/><Time issued="900" expiration="600"/></Lease></Leases>`), 1200, out)
	if out["0800278b3c9f"].ip != "10.0.2.15" {
		t.Fatalf("newest lease should win, got %v", out)
	}
}

func TestNICNumSort(t *testing.T) {
	if nicNum("nic10 (nat)") != 10 || nicNum("nic2 (hostonly)") != 2 || nicNum("") != 0 {
		t.Fatal("nicNum")
	}
}

func TestVMMACsSkipsDisabledNICs(t *testing.T) {
	info := map[string]string{
		"nic1": "hostonly", "macaddress1": "0800278B3C9F",
		"nic2": "nat", "macaddress2": "08002741E07C",
		"nic3": "none", "macaddress3": "080027000003",
	}
	got := vmMACs(info)
	if len(got) != 2 || got["0800278b3c9f"] != "nic1 (hostonly)" || got["08002741e07c"] != "nic2 (nat)" {
		t.Fatalf("got %v", got)
	}
}

func TestHardDisksAndPick(t *testing.T) {
	info := map[string]string{
		"SATA-0-0": `C:\VMs\a\a.vdi`, "SATA-ImageUUID-0-0": "uuid-a",
		"SATA-1-0": `C:\VMs\a\data.vmdk`, "SATA-ImageUUID-1-0": "uuid-b",
		"IDE-0-0": `C:\iso\ubuntu.iso`, "IDE-ImageUUID-0-0": "uuid-iso",
		"IDE-1-0": "emptydrive",
	}
	disks := hardDisks(info)
	if len(disks) != 2 || disks[0].uuid != "uuid-a" || disks[1].uuid != "uuid-b" {
		t.Fatalf("got %+v", disks)
	}
	if dk, ok := pickDisk(info, ""); !ok || dk.uuid != "uuid-a" {
		t.Fatalf("default pick = %+v", dk)
	}
	if dk, ok := pickDisk(info, "sata-1-0"); !ok || dk.uuid != "uuid-b" {
		t.Fatalf("slot pick = %+v", dk)
	}
	if _, ok := pickDisk(info, "uuid-iso"); ok {
		t.Fatal("ISO must not be picked as a hard disk")
	}
}

func TestMediumCapacity(t *testing.T) {
	if got := mediumCapacityMB("UUID: x\nCapacity:       25000 MBytes\nSize on disk: 3 MBytes"); got != 25000 {
		t.Fatalf("got %d", got)
	}
}

func TestUnattendedArgs(t *testing.T) {
	args := unattendedArgs("My Test VM", `C:\iso\u.iso`, "alice", `C:\tmp\pw`, "", "", true, "", "UTC", 0, "", "")
	joined := strings.Join(args, " ")
	for _, want := range []string{"--hostname=my-test-vm.local", "--full-user-name=alice", "--password-file=C:\\tmp\\pw",
		"--install-additions", "--time-zone=UTC", "--start-vm=headless"} {
		if !strings.Contains(joined, want) {
			t.Errorf("missing %s in %v", want, args)
		}
	}
	if strings.Contains(joined, "--password=") {
		t.Error("password must only travel via --password-file")
	}
}

func TestInaccessibleName(t *testing.T) {
	if !isInaccessible("<inaccessible>") || !isInaccessible("<inaccessible!>") || isInaccessible("GNS3 VM") {
		t.Fatal("isInaccessible wrong")
	}
}
