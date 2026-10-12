// Command itemsheet cuts Ironchest Games' CC0 RPG Items sheet into one PNG
// per item for the explorer:
//
//	go run ./cmd/itemsheet -src ~/Downloads/Ironchests_RPG_items_pico8_transparentbg_v1.png
package main

import (
	"flag"
	"image/png"
	"log"
	"os"
	"path/filepath"

	"github.com/memmaker/voxellib/items/sheet"
)

const credits = `Item sprites: "RPG Items" pico-8 sheet by Ironchest Games, CC0 1.0.
https://ironchestgames.itch.io  (source sheet: sheet.png)
`

func main() {
	src := flag.String("src", filepath.Join(os.Getenv("HOME"), "Downloads/Ironchests_RPG_items_pico8_transparentbg_v1.png"), "sprite sheet")
	out := flag.String("out", "assets/items", "output directory")
	flag.Parse()

	data, err := os.ReadFile(*src)
	if err != nil {
		log.Fatal(err)
	}
	f, _ := os.Open(*src)
	img, err := png.Decode(f)
	f.Close()
	if err != nil {
		log.Fatal(err)
	}
	if err := os.MkdirAll(*out, 0o755); err != nil {
		log.Fatal(err)
	}
	sprites := sheet.Cut(img, sheet.Ironchest)
	for _, s := range sprites {
		w, err := os.Create(filepath.Join(*out, s.Name+".png"))
		if err != nil {
			log.Fatal(err)
		}
		png.Encode(w, s.Image)
		w.Close()
	}
	os.WriteFile(filepath.Join(*out, "sheet.png"), data, 0o644)
	os.WriteFile(filepath.Join(*out, "CREDITS"), []byte(credits), 0o644)
	log.Printf("wrote %d items to %s", len(sprites), *out)
}
