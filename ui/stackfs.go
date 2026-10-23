package ui

import (
	"errors"
	"io/fs"
)

// StackFS layers file systems like a resource pack list: the first one
// that has a file wins (highest priority first).
type StackFS []fs.FS

func (s StackFS) Open(name string) (fs.File, error) {
	for _, f := range s {
		if file, err := f.Open(name); err == nil {
			return file, nil
		} else if !errors.Is(err, fs.ErrNotExist) {
			return nil, err
		}
	}
	return nil, &fs.PathError{Op: "open", Path: name, Err: fs.ErrNotExist}
}
