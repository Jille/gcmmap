# gcmmap

[![Go Reference](https://pkg.go.dev/badge/github.com/Jille/gcmmap.svg)](https://pkg.go.dev/github.com/Jille/gcmmap)

Package gcmmap provides mmap(2) that can be garbage collected by Go's garbage collector. There is no explicit munmap.

It works by allocating a []byte from the Go allocator, which we can set a finalizer on, and then using mmap with MAP_FIXED to overwrite it with your requested mmap. When the finalizer runs, we undo that and put a normal anonymous read/write mapping back.

USE AT YOUR OWN RISK.

## Impact on garbage collection

Contrary to normal `unix.Mmap()`s, memory mapped with gcmmap is considered to be part of Go's heap. [Go's garbage collector](https://go.dev/doc/gc-guide) by default (GOGC=100) targets to waste no more than another 100% of your heap size. This becomes especially relevant if you have file backed mappings that aren't fully in RSS.

We ran into trouble because we mmapped 20GB, but had only 8GB physical memory. The GC decided it would run the next garbage collection around a heap size of 40GB. The heap kept growing, the page cache kept shrinking and the machine came to a grinding halt. We worked around this by setting GOMEMLIMIT to 22GB, giving the Go heap basically 2GB to work with.
