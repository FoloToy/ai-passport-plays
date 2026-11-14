//go:build windows

package device

import (
	"regexp"
	"sort"
	"strings"
	"unsafe"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
)

var (
	cfgmgr32          = windows.NewLazySystemDLL("cfgmgr32.dll")
	cmLocateDevNodeW  = cfgmgr32.NewProc("CM_Locate_DevNodeW")
	comName           = regexp.MustCompile(`^COM\d+$`)
	enumUSB           = `SYSTEM\CurrentControlSet\Enum\USB`
	problemHardware   = `VID_0000&PID_0002`
	parentHardware    = `VID_303A&PID_1001`
	serialHardwareKey = `VID_303A&PID_1001&MI_00`
)

func present(instanceID string) bool {
	if cmLocateDevNodeW.Find() != nil {
		return false
	}
	p, err := windows.UTF16PtrFromString(instanceID)
	if err != nil {
		return false
	}
	var devinst uint32
	r, _, _ := cmLocateDevNodeW.Call(uintptr(unsafe.Pointer(&devinst)), uintptr(unsafe.Pointer(p)), 0)
	return r == 0 // CR_SUCCESS
}

func subkeys(path string) []string {
	k, err := registry.OpenKey(registry.LOCAL_MACHINE, path, registry.ENUMERATE_SUB_KEYS)
	if err != nil {
		return nil
	}
	defer k.Close()
	names, _ := k.ReadSubKeyNames(-1)
	return names
}

func stringValue(path, name string) string {
	k, err := registry.OpenKey(registry.LOCAL_MACHINE, path, registry.QUERY_VALUE)
	if err != nil {
		return ""
	}
	defer k.Close()
	v, _, _ := k.GetStringValue(name)
	return v
}

// problemPortHints maps a device stuck in Code 43 (VID_0000&PID_0002) back to the COM
// port its serial interface last used, so that port can be opened without enumeration.
func problemPortHints() []string {
	hints := map[string]bool{}
	for _, suffix := range subkeys(enumUSB + `\` + problemHardware) {
		if !present(`USB\` + problemHardware + `\` + suffix) {
			continue
		}
		prefix := stringValue(enumUSB+`\`+parentHardware+`\`+suffix, "ParentIdPrefix")
		if prefix == "" {
			continue
		}
		prefix = strings.ToLower(prefix) + "&"
		for _, inst := range subkeys(enumUSB + `\` + serialHardwareKey) {
			if !strings.HasPrefix(strings.ToLower(inst), prefix) {
				continue
			}
			name := strings.ToUpper(stringValue(enumUSB+`\`+serialHardwareKey+`\`+inst+`\Device Parameters`, "PortName"))
			if comName.MatchString(name) {
				hints[name] = true
			}
		}
	}
	out := make([]string, 0, len(hints))
	for h := range hints {
		out = append(out, h)
	}
	sort.Strings(out)
	return out
}
