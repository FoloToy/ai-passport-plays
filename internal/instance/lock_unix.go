//go:build !windows

package instance

import (
	"fmt"
	"os"
	"path/filepath"
	"syscall"
)

func Acquire(dir string) (func(), error) {
	f, e := os.OpenFile(filepath.Join(dir, "instance.lock"), os.O_CREATE|os.O_RDWR, 0600)
	if e != nil {
		return nil, e
	}
	if e = syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); e != nil {
		f.Close()
		return nil, fmt.Errorf("AI Passport 玩法社区已在运行，请切换回已有窗口")
	}
	return func() { _ = syscall.Flock(int(f.Fd()), syscall.LOCK_UN); _ = f.Close() }, nil
}
