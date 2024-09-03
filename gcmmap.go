// Package gcmmap provides mmap(2) that automatically munmaps when the garbage collector allows it.
//
// It uses unsafe and evil trickery. USE AT YOUR OWN RISK.
package gcmmap

import (
	"fmt"
	"runtime"
	"sync/atomic"
	"unsafe"

	"golang.org/x/sys/unix"
)

// NumActive is the number of mmaps that have not yet been garbage collected.
var NumActive atomic.Int32

// Mmap calls mmap(2) and relies on the garbage collector to munmap when dereferenced.
func Mmap(fd int, offset int64, len, prot, flags int) ([]byte, error) {
	// Do a regular allocation, which we can set a finalizer on.
	// Make it slightly larger than the requested length, to allow us to align to a page boundary, and to make sure we don't mmap over another allocation on the same page.
	container := make([]byte, (len+4096+4095) & ^4095)
	pageStart, skip := alignPointer(unsafe.Pointer(unsafe.SliceData(container)))

	addr, err := unix.MmapPtr(fd, offset, pageStart, uintptr(len), prot, flags|unix.MAP_FIXED)
	if err != nil {
		return nil, err
	}
	if addr != pageStart {
		panic("mmap(2) with MAP_FIXED chose a different address")
	}
	NumActive.Add(1)

	runtime.SetFinalizer(unsafe.SliceData(container), func(c *byte) {
		pageStart, _ := alignPointer(unsafe.Pointer(c))
		addr, err := unix.MmapPtr(-1, 0, pageStart, uintptr(len), unix.PROT_READ|unix.PROT_WRITE, unix.MAP_ANONYMOUS|unix.MAP_PRIVATE|unix.MAP_FIXED)
		if err != nil {
			panic(fmt.Errorf("Restoring normal memory failed when gcmmap got dereferenced: %w", err))
		}
		if addr != pageStart {
			panic("mmap(2) with MAP_FIXED chose a different address while restoring dereferenced gcmmap")
		}
		NumActive.Add(-1)
	})

	return container[skip:][:len], nil
}

func alignPointer(p unsafe.Pointer) (unsafe.Pointer, int) {
	o := uintptr(p) % 4096
	if o == 0 {
		return p, 0
	}
	return unsafe.Add(p, 4096-o), 4096 - int(o)
}
