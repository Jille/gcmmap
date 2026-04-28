// Package gcmmap provides mmap(2) that can be garbage collected by Go's garbage collector. There is no explicit munmap.
//
// It uses unsafe and evil trickery. USE AT YOUR OWN RISK.
package gcmmap

import (
	"fmt"
	"os"
	"runtime"
	"sync/atomic"
	"unsafe"

	"golang.org/x/sys/unix"

	// gcmmap would break with a moving garbage collector, so import this to ensure we refuse to start if built with a Go version with a moving garbage collector.
	_ "go4.org/unsafe/assume-no-moving-gc"
)

var (
	// NumActive is the number of mmaps that have not yet been garbage collected.
	NumActive atomic.Int32

	// BytesActive is the number of bytes across all mmaps that have not yet been garbage collected.
	BytesActive atomic.Int64

	seenUnalignedAlloc atomic.Bool
)

var pagesize = os.Getpagesize()

// Mmap calls mmap(2) and uses the garbage collector to unmap when no more references exist.
//
//go:nocheckptr
func Mmap(fd int, offset int64, len, prot, flags int) ([]byte, error) {
	// We use mallocgc to get memory tracked by the Go GC and mmap(2) over that.

	var container unsafe.Pointer
	if !seenUnalignedAlloc.Load() {
		// Allocate len rounded up to a page size to make sure we don't mmap over another allocation on the same page.
		container = allocateDirtyBytes((len + pagesize - 1) / pagesize * pagesize)
		if uintptr(container)%uintptr(pagesize) != 0 {
			// We got an allocation that didn't start on a page boundary. AFAIK that can't happen with current versions of Go.
			seenUnalignedAlloc.Store(true)
			container = nil
		}
	}
	if container == nil {
		// Allocate one page more than the requested length (rounded up to the page size), to allow us to align to a page boundary.
		container = allocateDirtyBytes((len + pagesize + pagesize - 1) / pagesize * pagesize)
	}
	pageStart := alignPointer(container)

	addr, err := unix.MmapPtr(fd, offset, pageStart, uintptr(len), prot, flags|unix.MAP_FIXED)
	if err != nil {
		return nil, err
	}
	if addr != pageStart {
		panic("mmap(2) with MAP_FIXED chose a different address")
	}
	NumActive.Add(1)
	BytesActive.Add(int64((len + pagesize - 1) / pagesize * pagesize))

	runtime.SetFinalizer((*byte)(container), func(container *byte) {
		// Unmapping can be slow, don't hold up the finalizer thread.
		go release(container, len)
	})

	return unsafe.Slice((*byte)(pageStart), len), nil
}

func alignPointer(p unsafe.Pointer) unsafe.Pointer {
	o := uintptr(p) % uintptr(pagesize)
	if o == 0 {
		return p
	}
	return unsafe.Add(p, pagesize-int(o))
}

// allocateDirtyBytes allocates bytes without zeroing them.
func allocateDirtyBytes(n int) unsafe.Pointer {
	return mallocgc(uintptr(n), nil, false)
}

//go:linkname mallocgc runtime.mallocgc
func mallocgc(size uintptr, typ unsafe.Pointer, needzero bool) unsafe.Pointer

func release(container *byte, length int) {
	// Make sure the memory is not reused until we're done with it.
	defer runtime.KeepAlive(container)

	pageStart := alignPointer(unsafe.Pointer(container))
	addr, err := unix.MmapPtr(-1, 0, pageStart, uintptr(length), unix.PROT_READ|unix.PROT_WRITE, unix.MAP_ANONYMOUS|unix.MAP_PRIVATE|unix.MAP_FIXED)
	if err != nil {
		panic(fmt.Errorf("Restoring normal memory failed when gcmmap got dereferenced: %w", err))
	}
	if addr != pageStart {
		panic("mmap(2) with MAP_FIXED chose a different address while garbage collecting")
	}
	NumActive.Add(-1)
	BytesActive.Add(-int64((length + pagesize - 1) / pagesize * pagesize))
}
