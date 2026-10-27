package instance

import (
	"os"
	"path/filepath"
)

type Lock struct{ f *os.File }

func Acquire(path string) (*Lock, error) {
	if e := os.MkdirAll(filepath.Dir(path), 0700); e != nil {
		return nil, e
	}
	f, e := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0600)
	if e != nil {
		return nil, e
	}
	if e = lock(f); e != nil {
		f.Close()
		return nil, e
	}
	return &Lock{f}, nil
}
func (l *Lock) Close() error { return l.f.Close() }
