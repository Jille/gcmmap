package gcmmap_test

import (
	"hash/crc32"
	"os"
	"runtime"
	"testing"
	"time"

	"github.com/Jille/gcmmap"
	"golang.org/x/sys/unix"
)

func TestSimple(t *testing.T) {
	fh, err := os.Open("/bin/sh")
	if err != nil {
		t.Fatalf("Failed to open: %v", err)
	}
	st, err := fh.Stat()
	if err != nil {
		t.Fatalf("Failed to stat: %v", err)
	}
	b, err := gcmmap.Mmap(int(fh.Fd()), 0, int(st.Size()), unix.PROT_READ, unix.MAP_SHARED)
	if err != nil {
		t.Fatalf("Failed to mmap: %v", err)
	}
	if gcmmap.NumActive.Load() != 1 {
		t.Errorf("NumActive isn't 1 after a single Mmap")
	}
	_ = crc32.ChecksumIEEE(b)
	b = nil
	runtime.GC()
	for i := 0; ; i++ {
		if gcmmap.NumActive.Load() == 0 {
			break
		}
		if i == 1000 {
			t.Fatal("GC didn't call finalizer")
		}
		runtime.GC()
		time.Sleep(time.Millisecond)
	}
}
