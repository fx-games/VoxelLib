package ui

import (
	"errors"
	"io/fs"
)

// StackFS layers file systems like a resource pack list: the first one
// that has a file wins (highest priority first). It is how a game puts
// its own assets over the client jar's:
//
//	jar, _ := zip.OpenReader(path)
//	ctx, _ := ui.NewContext(canvas, ui.StackFS{os.DirFS("mypack"), jar}, lang)
//	ctx.Tex("textures/items/sword.png") // mypack/assets/minecraft/textures/items/sword.png
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
