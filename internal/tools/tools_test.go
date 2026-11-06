package tools

import (
	"strings"
	"testing"
)

func TestIsLegacyOS(t *testing.T) {
	legacy := []string{"WindowsXP", "WindowsXP_64", "Windows2000", "Windows98", "DOS", "WindowsNT4"}
	for _, os := range legacy {
		if !isLegacyOS(os) {
			t.Errorf("%s should be legacy", os)
		}
	}
	modern := []string{"Windows10_64", "Ubuntu_64", "Windows7_64", "Fedora_64"}
	for _, os := range modern {
		if isLegacyOS(os) {
			t.Errorf("%s should not be legacy", os)
		}
	}
}

func TestGetOSDefaults(t *testing.T) {
	cases := map[string]osDefaults{
		"WindowsXP":    {512, 1, 10000},
		"Windows10_64": {4096, 2, 50000},
		"Ubuntu_64":    {2048, 2, 25000},
		"DOS":          {64, 1, 2000},
		"UnknownOS":    {2048, 2, 40000},
	}
	for os, want := range cases {
		if got := getOSDefaults(os); got != want {
			t.Errorf("getOSDefaults(%s) = %+v, want %+v", os, got, want)
		}
	}
}

func TestParseOSTypesCombined(t *testing.T) {
	in := "ID / Description: WindowsXP -- Windows XP\nID / Description: Ubuntu_64 -- Ubuntu (64-bit)\n"
	got := parseOSTypes(in)
	if len(got) != 2 {
		t.Fatalf("got %d types, want 2", len(got))
	}
	if got[0].ID != "WindowsXP" || got[0].Description != "Windows XP" {
		t.Errorf("first = %+v", got[0])
	}
}

func TestParseOSTypesLegacyFormat(t *testing.T) {
	in := "ID:          WindowsXP\nDescription: Windows XP\n\nID:          Ubuntu_64\nDescription: Ubuntu (64-bit)\n"
	got := parseOSTypes(in)
	if len(got) != 2 {
		t.Fatalf("got %d types, want 2", len(got))
	}
	if got[1].ID != "Ubuntu_64" || got[1].Description != "Ubuntu (64-bit)" {
		t.Errorf("second = %+v", got[1])
	}
}

func TestLookupGuide(t *testing.T) {
	cases := map[string]string{
		"xp":         "Windows XP Professional SP3",
		"windowsxp":  "Windows XP Professional SP3",
		"win7":       "Windows 7",
		"win11":      "Windows 10/11",
		"linux":      "Ubuntu Linux",
		"Windows XP": "Windows XP Professional SP3",
	}
	for in, wantDisplay := range cases {
		g, ok := lookupGuide(in)
		if !ok {
			t.Errorf("lookupGuide(%q) not found", in)
			continue
		}
		if g.displayName != wantDisplay {
			t.Errorf("lookupGuide(%q) = %q, want %q", in, g.displayName, wantDisplay)
		}
	}
	if _, ok := lookupGuide("plan9"); ok {
		t.Error("plan9 should not have a guide")
	}
}

func TestRenderGuideContainsSections(t *testing.T) {
	g, _ := lookupGuide("windowsxp")
	out := renderGuide(g)
	for _, section := range []string{"Recommended Settings", "Installation Steps", "Gotchas", "Post-Install", "vm_guest_exec Usage"} {
		if !strings.Contains(out, section) {
			t.Errorf("rendered guide missing section %q", section)
		}
	}
}
