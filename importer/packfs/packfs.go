// Package packfs lists the files of an asset pack, whatever form it was
// dropped in: a folder, a .zip, zips nested in zips or folders, or a single
// file. Importers use it so they never care about the container.
package packfs

import (
	"archive/zip"
	"bytes"
	"io"
	"os"
	"path"
	"path/filepath"
	"strings"
)

// File is one entry, read lazily.
type File struct {
	Name string // slash-separated path inside the pack
	Read func() ([]byte, error)
}

// List returns every file in src.
func List(src string) ([]File, error) {
	st, err := os.Stat(src)
	if err != nil {
		return nil, err
	}
	if st.IsDir() {
		var out []File
		err := filepath.WalkDir(src, func(p string, d os.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() {
				if d.Name() == ".git" {
					return filepath.SkipDir
				}
				return nil
			}
			rel, _ := filepath.Rel(src, p)
			rel = filepath.ToSlash(rel)
			if strings.EqualFold(filepath.Ext(p), ".zip") {
				data, err := os.ReadFile(p)
				if err != nil {
					return err
				}
				sub, err := listZip(data, rel)
				if err != nil {
					return err
				}
				out = append(out, sub...)
				return nil
			}
			out = append(out, File{rel, func() ([]byte, error) { return os.ReadFile(p) }})
			return nil
		})
		return out, err
	}
	if strings.EqualFold(filepath.Ext(src), ".zip") {
		data, err := os.ReadFile(src)
		if err != nil {
			return nil, err
		}
		return listZip(data, "")
	}
	return []File{{filepath.Base(src), func() ([]byte, error) { return os.ReadFile(src) }}}, nil
}

func listZip(data []byte, prefix string) ([]File, error) {
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return nil, err
	}
	var out []File
	for _, zf := range zr.File {
		if zf.FileInfo().IsDir() || strings.HasPrefix(path.Base(zf.Name), "._") || strings.HasPrefix(zf.Name, "__MACOSX/") {
			continue
		}
		zf := zf
		read := func() ([]byte, error) {
			r, err := zf.Open()
			if err != nil {
				return nil, err
			}
			defer r.Close()
			return io.ReadAll(r)
		}
		name := path.Join(prefix, zf.Name)
		if strings.EqualFold(path.Ext(zf.Name), ".zip") {
			inner, err := read()
			if err != nil {
				return nil, err
			}
			sub, err := listZip(inner, name)
			if err != nil {
				return nil, err
			}
			out = append(out, sub...)
			continue
		}
		out = append(out, File{name, read})
	}
	return out, nil
}
