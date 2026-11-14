//go:build !windows

package syncer

import (
	"os"
	"path/filepath"
	"syscall"
)

// freeSpace reports available bytes on the volume holding dir (or its nearest existing parent).
func freeSpace(dir string) (uint64, bool) {
	for {
		var st syscall.Statfs_t
		if err := syscall.Statfs(dir, &st); err == nil {
			return uint64(st.Bavail) * uint64(st.Bsize), true
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return 0, false
		}
		if _, err := os.Stat(dir); err == nil {
			return 0, false
		}
		dir = parent
	}
}

// FreeSpace is exported for the settings page.
func FreeSpace(dir string) (uint64, bool) { return freeSpace(dir) }
