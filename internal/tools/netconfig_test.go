package tools

import (
	"reflect"
	"testing"
)

func TestParseNamedList(t *testing.T) {
	bridged := `Name:            wlan0
GUID:            abc
Name:            eth0
DHCP:            Disabled
`
	got := parseNamedList(bridged, "Name")
	if want := []string{"wlan0", "eth0"}; !reflect.DeepEqual(got, want) {
		t.Errorf("bridged Names = %v, want %v", got, want)
	}

	natnets := "NetworkName:    NatNetwork\nNetworkName:    LabNet\n"
	if got := parseNamedList(natnets, "NetworkName"); !reflect.DeepEqual(got, []string{"NatNetwork", "LabNet"}) {
		t.Errorf("natnetwork names = %v", got)
	}

	if got := parseNamedList("", "Name"); len(got) != 0 {
		t.Errorf("empty input should yield no names, got %v", got)
	}
}

func TestParseNamedListDedup(t *testing.T) {
	in := "Name:  hostonly0\nName:  hostonly0\nName:  hostonly1\n"
	if got := parseNamedList(in, "Name"); !reflect.DeepEqual(got, []string{"hostonly0", "hostonly1"}) {
		t.Errorf("dedup failed: %v", got)
	}
}

// TestReadOnlySafeToolsAreReal guards against typos: every allow-listed tool
// name must be one the server actually registers.
func TestReadOnlySafeToolsExist(t *testing.T) {
	known := map[string]bool{
		"vm_list": true, "vm_info": true, "vm_screen_info": true, "vm_screenshot": true,
		"vm_list_ostypes": true, "vm_install_guide": true, "vm_get_guest_ip": true,
		"vm_list_host_networks": true, "vm_wait_for_guest_additions": true, "vm_wait_for_network": true,
		"vm_wait_for_state": true, "vm_logs": true, "vm_wait_for_port": true,
	}
	for name := range readOnlySafeTools {
		if !known[name] {
			t.Errorf("read-only allow-list has unknown tool %q", name)
		}
	}
}
