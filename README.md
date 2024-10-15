# gcmmap

[![Go Reference](https://pkg.go.dev/badge/github.com/Jille/gcmmap.svg)](https://pkg.go.dev/github.com/Jille/gcmmap)

Package gcmmap provides mmap(2) that can be garbage collected by Go's garbage collector. There is no explicit munmap.

It works by allocating a []byte from the Go allocator, which we can set a finalizer on, and then using mmap with MAP_FIXED to overwrite it with your requested mmap. When the finalizer runs, we undo that and put a normal anonymous read/write mapping back.

USE AT YOUR OWN RISK.
