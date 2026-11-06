package vbox

import (
	"regexp"
	"strings"
)

// VM is a name/UUID pair as reported by `VBoxManage list vms`.
type VM struct {
	Name string `json:"name"`
	UUID string `json:"uuid"`
}

// ParseMachineReadable parses `--machinereadable` output (key="value" lines)
// into a map. Surrounding quotes are stripped and escaped backslashes ("\\")
// are unescaped, matching VBoxManage's Windows path encoding.
func ParseMachineReadable(output string) map[string]string {
	result := make(map[string]string)
	for _, line := range strings.Split(output, "\n") {
		trimmed := strings.TrimSpace(line)
		eq := strings.IndexByte(trimmed, '=')
		if trimmed == "" || eq < 0 {
			continue
		}
		key := strings.Trim(trimmed[:eq], `"`)
		val := strings.Trim(trimmed[eq+1:], `"`)
		val = strings.ReplaceAll(val, `\\`, `\`)
		result[key] = val
	}
	return result
}

var vmListRe = regexp.MustCompile(`^"(.+)"\s+\{(.+)\}$`)

// ParseVMList parses `"name" {uuid}` lines from `VBoxManage list vms`.
func ParseVMList(output string) []VM {
	var vms []VM
	for _, line := range strings.Split(output, "\n") {
		if m := vmListRe.FindStringSubmatch(strings.TrimSpace(line)); m != nil {
			vms = append(vms, VM{Name: m[1], UUID: m[2]})
		}
	}
	return vms
}
