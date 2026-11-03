// Command importbedrock converts a Minecraft Bedrock Edition world (a
// .mcworld or .zip, or its unpacked folder) into a VoxelMap, with the
// models and textures of a Java Edition client jar:
//
//	importbedrock -in world.zip -jar client.jar -out maps/minecity
//
// The map holds the jar's assets, so keep it to yourself (maps/ is
// ignored by git): every player imports it from the same world.
package main

import (
	"archive/zip"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/memmaker/voxellib/importer/bedrock"
)

func main() {
	in := flag.String("in", "", "Bedrock world: .mcworld, .zip or folder")
	jar := flag.String("jar", "", "Java Edition client jar (models and textures)")
	out := flag.String("out", "", "VoxelMap directory to write (e.g. maps/minecity)")
	name := flag.String("name", "", "map name (default: the world's)")
	flag.Parse()
	if *in == "" || *jar == "" || *out == "" {
		flag.Usage()
		os.Exit(2)
	}
	dir := *in
	if st, err := os.Stat(dir); err != nil {
		log.Fatal(err)
	} else if !st.IsDir() {
		tmp, err := os.MkdirTemp("", "importbedrock")
		if err != nil {
			log.Fatal(err)
		}
		defer os.RemoveAll(tmp)
		if dir, err = unzip(*in, tmp); err != nil {
			log.Fatal(err)
		}
	} else if d, err := findWorld(dir); err == nil {
		dir = d
	}
	w, err := bedrock.Open(dir)
	if err != nil {
		log.Fatal(err)
	}
	log.Printf("%s: %d keys", w.Name, w.DB.Len())
	if err := bedrock.Export(w, bedrock.ExportOptions{Jar: *jar, Out: *out, Name: *name, Logf: log.Printf}); err != nil {
		log.Fatal(err)
	}
	fmt.Println("wrote", *out)
}

// unzip unpacks a world archive into dir and returns the folder holding
// its level.dat.
func unzip(file, dir string) (string, error) {
	z, err := zip.OpenReader(file)
	if err != nil {
		return "", err
	}
	defer z.Close()
	for _, f := range z.File {
		dst := filepath.Join(dir, filepath.FromSlash(f.Name))
		if !strings.HasPrefix(dst, filepath.Clean(dir)+string(os.PathSeparator)) {
			return "", fmt.Errorf("%s: bad path %q", file, f.Name)
		}
		if f.FileInfo().IsDir() {
			continue
		}
		if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
			return "", err
		}
		r, err := f.Open()
		if err != nil {
			return "", err
		}
		o, err := os.Create(dst)
		if err == nil {
			_, err = io.Copy(o, r)
			o.Close()
		}
		r.Close()
		if err != nil {
			return "", err
		}
	}
	return findWorld(dir)
}

// findWorld returns dir or the first folder below it with a level.dat.
func findWorld(dir string) (string, error) {
	var found string
	filepath.WalkDir(dir, func(p string, d os.DirEntry, err error) error {
		if err == nil && found == "" && d.Name() == "level.dat" {
			found = filepath.Dir(p)
		}
		return nil
	})
	if found == "" {
		return "", fmt.Errorf("%s: no level.dat", dir)
	}
	return found, nil
}
