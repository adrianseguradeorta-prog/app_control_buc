package instance

import (
	"os"
	"path/filepath"
	"testing"
)

func TestExclusiveAndStaleFile(t *testing.T) {
	p := filepath.Join(t.TempDir(), "path with spaces", "server.lock")
	a, e := Acquire(p)
	if e != nil {
		t.Fatal(e)
	}
	if b, e := Acquire(p); e == nil {
		b.Close()
		t.Fatal("double lock")
	}
	a.Close()
	if _, e = os.Stat(p); e != nil {
		t.Fatal(e)
	}
	b, e := Acquire(p)
	if e != nil {
		t.Fatal("stale file blocks restart", e)
	}
	b.Close()
}
