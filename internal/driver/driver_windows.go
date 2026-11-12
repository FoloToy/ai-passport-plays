//go:build windows

package driver

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
)

const serialHardware = `VID_303A&PID_1001&MI_00`

var (
	cfgmgr32               = windows.NewLazySystemDLL("cfgmgr32.dll")
	cmLocateDevNodeW       = cfgmgr32.NewProc("CM_Locate_DevNodeW")
	cmGetDevNodeStatus     = cfgmgr32.NewProc("CM_Get_DevNode_Status")
	cmProblemFailedInstall = uintptr(28) // CM_PROB_FAILED_INSTALL: no driver
	dnHasProblem           = uint32(0x00000400)
)

// problem returns the device manager problem code of a present device, or -1 when the
// device is not plugged in.
func problem(instanceID string) int {
	if cmLocateDevNodeW.Find() != nil || cmGetDevNodeStatus.Find() != nil {
		return -1
	}
	p, err := windows.UTF16PtrFromString(instanceID)
	if err != nil {
		return -1
	}
	var devinst uint32
	if r, _, _ := cmLocateDevNodeW.Call(uintptr(unsafe.Pointer(&devinst)), uintptr(unsafe.Pointer(p)), 0); r != 0 {
		return -1
	}
	var status, prob uint32
	if r, _, _ := cmGetDevNodeStatus.Call(uintptr(unsafe.Pointer(&status)), uintptr(unsafe.Pointer(&prob)), uintptr(devinst), 0); r != 0 {
		return -1
	}
	if status&dnHasProblem == 0 {
		return 0
	}
	return int(prob)
}

// Check reports Missing when a plugged-in AI Passport's serial interface has no driver.
func Check() string {
	k, err := registry.OpenKey(registry.LOCAL_MACHINE, `SYSTEM\CurrentControlSet\Enum\USB\`+serialHardware, registry.ENUMERATE_SUB_KEYS)
	if err != nil {
		return OK
	}
	defer k.Close()
	names, _ := k.ReadSubKeyNames(-1)
	for _, inst := range names {
		code := problem(`USB\` + serialHardware + `\` + inst)
		if code == int(cmProblemFailedInstall) || code == 1 /* CM_PROB_NOT_CONFIGURED */ || code == 10 /* failed to start */ {
			return Missing
		}
	}
	return OK
}

func pnputil() string {
	windir := os.Getenv("SystemRoot")
	if windir == "" {
		windir = `C:\Windows`
	}
	// A 32-bit process on 64-bit Windows must use Sysnative to reach the 64-bit tool.
	if p := filepath.Join(windir, "Sysnative", "pnputil.exe"); fileExists(p) {
		return p
	}
	return filepath.Join(windir, "System32", "pnputil.exe")
}

func fileExists(p string) bool {
	st, err := os.Stat(p)
	return err == nil && !st.IsDir()
}

// Install writes the INF to dir and installs it on matching devices with pnputil,
// asking for administrator rights through UAC. It blocks until pnputil finishes.
func Install(dir string) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	inf := filepath.Join(dir, InfName)
	if err := os.WriteFile(inf, INF, 0o644); err != nil {
		return err
	}
	quote := func(s string) string { return "'" + strings.ReplaceAll(s, "'", "''") + "'" }
	// Windows 7 ships PowerShell 2.0, which already has Start-Process -Verb RunAs -Wait.
	script := fmt.Sprintf(`$p = Start-Process -FilePath %s -ArgumentList %s -Verb RunAs -Wait -PassThru; exit $p.ExitCode`,
		quote(pnputil()), quote(`-i -a "`+inf+`"`))
	cmd := exec.Command("powershell.exe", "-NoProfile", "-NonInteractive", "-ExecutionPolicy", "Bypass", "-Command", script)
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	out, err := cmd.CombinedOutput()
	if err != nil {
		msg := strings.TrimSpace(string(out))
		if msg == "" {
			msg = err.Error()
		}
		return fmt.Errorf("driver install failed or was cancelled: %s", msg)
	}
	return nil
}
