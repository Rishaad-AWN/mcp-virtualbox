package vbox

import (
	"errors"
	"testing"
)

func TestParseMachineReadable(t *testing.T) {
	in := "name=\"MyVM\"\nmemory=2048\nCfgFile=\"C:\\\\VMs\\\\MyVM\\\\MyVM.vbox\"\n\ninvalidline\n"
	got := ParseMachineReadable(in)

	if got["name"] != "MyVM" {
		t.Errorf("name = %q, want MyVM", got["name"])
	}
	if got["memory"] != "2048" {
		t.Errorf("memory = %q, want 2048", got["memory"])
	}
	if got["CfgFile"] != `C:\VMs\MyVM\MyVM.vbox` {
		t.Errorf("CfgFile = %q, want unescaped backslashes", got["CfgFile"])
	}
	if _, ok := got["invalidline"]; ok {
		t.Error("line without '=' should be skipped")
	}
}

func TestParseVMList(t *testing.T) {
	in := `"Windows XP" {12345678-1234-1234-1234-123456789abc}
"Ubuntu" {abcdef01-0000-0000-0000-000000000000}
garbage line`
	got := ParseVMList(in)
	if len(got) != 2 {
		t.Fatalf("got %d vms, want 2", len(got))
	}
	if got[0].Name != "Windows XP" || got[0].UUID != "12345678-1234-1234-1234-123456789abc" {
		t.Errorf("first vm = %+v", got[0])
	}
	if got[1].Name != "Ubuntu" {
		t.Errorf("second vm name = %q", got[1].Name)
	}
}

func TestFormatError(t *testing.T) {
	ve := newVBoxError("startvm", "code VBOX_E_INVALID_OBJECT_STATE\nmore", "VBoxManage startvm failed: bad state")
	if ve.Code != "VBOX_E_INVALID_OBJECT_STATE" {
		t.Errorf("code = %q", ve.Code)
	}
	if got := FormatError(ve); got != "[VBOX_E_INVALID_OBJECT_STATE] VBoxManage startvm failed: bad state" {
		t.Errorf("FormatError = %q", got)
	}
	if got := FormatError(errors.New("plain")); got != "plain" {
		t.Errorf("plain error = %q", got)
	}
	if got := FormatError(nil); got != "" {
		t.Errorf("nil = %q, want empty", got)
	}
}
