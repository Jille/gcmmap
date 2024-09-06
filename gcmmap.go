// Package gcmmap provides mmap(2) that can be garbage collected by Go's garbage collector. There is no explicit munmap.
//
// It uses unsafe and evil trickery. USE AT YOUR OWN RISK.
package gcmmap

import (
	"fmt"
	"runtime"
	"sync/atomic"
	"unsafe"

	"golang.org/x/sys/unix"

	// gcmmap would break with a moving garbage collector, so import this to ensure we refuse to start if built with a Go version with a moving garbage collector.
	_ "go4.org/unsafe/assume-no-moving-gc"
)

// NumActive is the number of mmaps that have not yet been garbage collected.
var NumActive atomic.Int32

// Mmap calls mmap(2) and uses the garbage collector to unmap when no more references exist.
func Mmap(fd int, offset int64, len, prot, flags int) ([]byte, error) {
	// Do a regular allocation, which we can set a finalizer on.
	// Make it slightly larger than the requested length, to allow us to align to a page boundary, and to make sure we don't mmap over another allocation on the same page.
	container := allocateDirtyBytes((len + 4096 + 4095) & ^4095)
	pageStart := alignPointer(container)

	addr, err := unix.MmapPtr(fd, offset, pageStart, uintptr(len), prot, flags|unix.MAP_FIXED)
	if err != nil {
		return nil, err
	}
	if addr != pageStart {
		panic("mmap(2) with MAP_FIXED chose a different address")
	}
	NumActive.Add(1)

	runtime.SetFinalizer((*byte)(container), func(container *byte) {
		pageStart := alignPointer(unsafe.Pointer(container))
		addr, err := unix.MmapPtr(-1, 0, pageStart, uintptr(len), unix.PROT_READ|unix.PROT_WRITE, unix.MAP_ANONYMOUS|unix.MAP_PRIVATE|unix.MAP_FIXED)
		if err != nil {
			panic(fmt.Errorf("Restoring normal memory failed when gcmmap got dereferenced: %w", err))
		}
		if addr != pageStart {
			panic("mmap(2) with MAP_FIXED chose a different address while garbage collecting")
		}
		NumActive.Add(-1)
	})

	return unsafe.Slice((*byte)(pageStart), len), nil
}

func alignPointer(p unsafe.Pointer) unsafe.Pointer {
	o := uintptr(p) % 4096
	if o == 0 {
		return p
	}
	return unsafe.Add(p, 4096-o)
}

// allocateDirtyBytes allocates bytes without zeroing them.
func allocateDirtyBytes(n int) unsafe.Pointer {
	return mallocgc(uintptr(n), nil, false)
}

//go:linkname mallocgc runtime.mallocgc
func mallocgc(size uintptr, typ unsafe.Pointer, needzero bool) unsafe.Pointer
