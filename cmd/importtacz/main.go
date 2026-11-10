// Command importtacz converts the TACZ default gun pack into assets/tacz:
//
//	importtacz -mod <TACZ checkout>/src/main/resources/assets/tacz -out assets/tacz
//
// -mod is the mod's assets/tacz folder; the pack is its custom/tacz_default_gun.
// It writes guns.json (weapon.Pack), models/<gun>.gltf, icons/gun|ammo/<id>.png,
// and the guns' sounds as an audio pack (sounds.json, tacz/sounds/...).
package main

import (
	"encoding/json"
	"flag"
	"image"
	"image/png"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/memmaker/voxellib/importer/tacz"
	"github.com/memmaker/voxellib/model/gltf"
	"github.com/memmaker/voxellib/weapon"
)

func main() {
	mod := flag.String("mod", "", "the TACZ mod's assets/tacz folder")
	out := flag.String("out", "assets/tacz", "directory to write")
	flag.Parse()
	if *mod == "" {
		flag.Usage()
		os.Exit(2)
	}
	packDir := filepath.Join(*mod, "custom", "tacz_default_gun")
	r, err := tacz.Import(os.DirFS(packDir), os.DirFS(*mod))
	if err != nil {
		log.Fatal(err)
	}
	if err := os.RemoveAll(*out); err != nil {
		log.Fatal(err)
	}
	var p weapon.Pack
	for _, g := range r.Guns {
		check(gltf.WriteFile(filepath.Join(*out, g.Spec.Model), g.Model))
		check(writePNG(filepath.Join(*out, g.Spec.Icon), g.Icon))
		p.Guns = append(p.Guns, g.Spec)
	}
	for _, a := range r.Ammo {
		check(writePNG(filepath.Join(*out, a.Icon), r.AmmoIcons[a.ID]))
		p.Ammo = append(p.Ammo, a)
	}
	check(writeJSON(filepath.Join(*out, "guns.json"), p))

	type event struct {
		Category string   `json:"category"`
		Sounds   []string `json:"sounds"`
	}
	events := map[string]event{}
	for ev, file := range r.Sounds {
		ns, path, _ := strings.Cut(ev, ":")
		check(copyFile(filepath.Join(packDir, file), filepath.Join(*out, ns, "sounds", path+".ogg")))
		events[ev] = event{"player", []string{ev}}
	}
	check(writeJSON(filepath.Join(*out, "sounds.json"), events))
	log.Printf("%d guns, %d ammo types, %d sounds; %d sound cues name no file: %v",
		len(p.Guns), len(p.Ammo), len(r.Sounds), len(r.Missing), r.Missing)
}

func check(err error) {
	if err != nil {
		log.Fatal(err)
	}
}

func writeJSON(path string, v any) error {
	data, err := json.MarshalIndent(v, "", "\t")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(data, '\n'), 0o644)
}

func writePNG(path string, img image.Image) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	if err := png.Encode(f, img); err != nil {
		f.Close()
		return err
	}
	return f.Close()
}

func copyFile(src, dst string) error {
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		return err
	}
	return out.Close()
}
