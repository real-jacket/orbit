//go:build darwin

package scanner

import (
	"os"
	"runtime"
	"sync"
	"syscall"
	"unsafe"

	"github.com/ebitengine/purego"
)

var exclusiveRename struct {
	sync.Once
	call uintptr
	err  error
}

// RENAME_EXCL closes the exists/rename race with Finder or another process.
func renameWithoutOverwrite(from, to string) error {
	fromPointer, err := syscall.BytePtrFromString(from)
	if err != nil {
		return &os.LinkError{Op: "rename", Old: from, New: to, Err: err}
	}
	toPointer, err := syscall.BytePtrFromString(to)
	if err != nil {
		return &os.LinkError{Op: "rename", Old: from, New: to, Err: err}
	}
	exclusiveRename.Do(func() {
		library, err := purego.Dlopen("/usr/lib/libSystem.B.dylib", purego.RTLD_NOW|purego.RTLD_LOCAL)
		if err != nil {
			exclusiveRename.err = err
			return
		}
		exclusiveRename.call, exclusiveRename.err = purego.Dlsym(library, "renamex_np")
	})
	if exclusiveRename.err != nil {
		return exclusiveRename.err
	}
	result, _, errno := purego.SyscallN(exclusiveRename.call, uintptr(unsafe.Pointer(fromPointer)), uintptr(unsafe.Pointer(toPointer)), 0x00000004)
	runtime.KeepAlive(fromPointer)
	runtime.KeepAlive(toPointer)
	if result == 0 {
		return nil
	}
	return &os.LinkError{Op: "rename", Old: from, New: to, Err: syscall.Errno(uint32(errno))}
}
