package resloc

import (
	"encoding/json"
	"io/fs"
	"os"
	"path/filepath"
)

// AssetIndex is a launcher asset index (assets/indexes/<version>.json,
// whose objects live in assets/objects/<hh>/<hash>) as a file system:
// each index key ("minecraft/sounds/random/explode1.ogg") opens its
// object. Keys whose object is not on disk do not exist.
func AssetIndex(index string) (fs.FS, error) {
	data, err := os.ReadFile(index)
	if err != nil {
		return nil, err
	}
	var idx struct {
		Objects map[string]struct{ Hash string }
	}
	if err := json.Unmarshal(data, &idx); err != nil {
		return nil, err
	}
	objects := filepath.Join(filepath.Dir(filepath.Dir(index)), "objects")
	a := assetFS{}
	for name, o := range idx.Objects {
		if len(o.Hash) > 2 {
			a[name] = filepath.Join(objects, o.Hash[:2], o.Hash)
		}
	}
	return a, nil
}

// assetFS maps index keys to object files.
type assetFS map[string]string

func (a assetFS) Open(name string) (fs.File, error) {
	p, ok := a[name]
	if !ok {
		return nil, &fs.PathError{Op: "open", Path: name, Err: fs.ErrNotExist}
	}
	return os.Open(p)
}
