package instance

import (
	"os"
	"syscall"
	"unsafe"
)

var lockFileEx = syscall.NewLazyDLL("kernel32.dll").NewProc("LockFileEx")

func lock(f *os.File) error {
	var o syscall.Overlapped
	r, _, e := lockFileEx.Call(f.Fd(), 3, 0, 1, 0, uintptr(unsafe.Pointer(&o)))
	if r == 0 {
		return e
	}
	return nil
}
