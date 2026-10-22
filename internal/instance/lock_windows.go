package instance

import (
	"fmt"
	"os"
	"path/filepath"
	"syscall"
	"unsafe"
)

func Acquire(dir string) (func(), error) {
	f, e := os.OpenFile(filepath.Join(dir, "instance.lock"), os.O_CREATE|os.O_RDWR, 0600)
	if e != nil {
		return nil, e
	}
	over := new(syscall.Overlapped)
	lock := syscall.NewLazyDLL("kernel32.dll").NewProc("LockFileEx")
	unlock := syscall.NewLazyDLL("kernel32.dll").NewProc("UnlockFileEx")
	ok, _, _ := lock.Call(f.Fd(), 3, 0, 1, 0, uintptr(unsafe.Pointer(over)))
	if ok == 0 {
		f.Close()
		return nil, fmt.Errorf("AI Passport 玩法社区已在运行，请切换回已有窗口")
	}
	return func() { unlock.Call(f.Fd(), 0, 1, 0, uintptr(unsafe.Pointer(over))); _ = f.Close() }, nil
}
