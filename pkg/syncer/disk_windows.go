package syncer

import (
	"os"
	"path/filepath"
	"syscall"
	"unsafe"
)

var getDiskFreeSpaceEx = syscall.NewLazyDLL("kernel32.dll").NewProc("GetDiskFreeSpaceExW")

// freeSpace reports available bytes on the volume holding dir (or its nearest existing parent).
func freeSpace(dir string) (uint64, bool) {
	for {
		if _, err := os.Stat(dir); err == nil {
			p, err := syscall.UTF16PtrFromString(dir)
			if err != nil {
				return 0, false
			}
			var free uint64
			ok, _, _ := getDiskFreeSpaceEx.Call(uintptr(unsafe.Pointer(p)), uintptr(unsafe.Pointer(&free)), 0, 0)
			return free, ok != 0
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return 0, false
		}
		dir = parent
	}
}

// FreeSpace is exported for the settings page.
func FreeSpace(dir string) (uint64, bool) { return freeSpace(dir) }
