// Command fetchskins downloads Minecraft player skins for the multiplayer
// demo: it tries random dictionary-style player names against the Mojang API
// and saves the first -n with a custom 64x64 skin as <name>.png in -out.
// Skins belong to their players; the folder stays out of git.
//
//	go run ./cmd/fetchskins -n 2 -out skins
package main

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"flag"
	"fmt"
	"image/png"
	"io"
	"log"
	"math/rand/v2"
	"net/http"
	"os"
	"path/filepath"
	"time"
)

var words = []string{"Arrow", "Badger", "Cobalt", "Dusk", "Ember", "Falcon", "Granite", "Hollow", "Ivy", "Jasper",
	"Kestrel", "Lumen", "Maple", "Nimbus", "Onyx", "Pebble", "Quartz", "Raven", "Saffron", "Thistle", "Umber",
	"Vesper", "Willow", "Yarrow", "Zephyr", "Comet", "Fern", "Mossy", "Pine", "Storm"}

var client = &http.Client{Timeout: 10 * time.Second}

func getJSON(url string, v any) error {
	resp, err := client.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("%s: %s", url, resp.Status)
	}
	return json.NewDecoder(resp.Body).Decode(v)
}

// skinURL returns the player's skin texture URL ("" for none) and whether
// it uses the slim (Alex) arms.
func skinURL(name string) (string, bool, error) {
	var prof struct{ ID, Name string }
	if err := getJSON("https://api.mojang.com/users/profiles/minecraft/"+name, &prof); err != nil {
		return "", false, err
	}
	var sess struct {
		Properties []struct{ Name, Value string }
	}
	if err := getJSON("https://sessionserver.mojang.com/session/minecraft/profile/"+prof.ID, &sess); err != nil {
		return "", false, err
	}
	for _, p := range sess.Properties {
		if p.Name != "textures" {
			continue
		}
		raw, err := base64.StdEncoding.DecodeString(p.Value)
		if err != nil {
			return "", false, err
		}
		var tex struct {
			Textures struct {
				Skin struct {
					URL      string
					Metadata struct{ Model string }
				} `json:"SKIN"`
			}
		}
		if err := json.Unmarshal(raw, &tex); err != nil {
			return "", false, err
		}
		return tex.Textures.Skin.URL, tex.Textures.Skin.Metadata.Model == "slim", nil
	}
	return "", false, nil
}

func main() {
	n := flag.Int("n", 2, "skins to fetch")
	out := flag.String("out", "skins", "output directory")
	slim := flag.Bool("slim", false, "also accept slim-armed (Alex) skins")
	flag.Parse()
	if err := os.MkdirAll(*out, 0o755); err != nil {
		log.Fatal(err)
	}
	got := 0
	for _, i := range rand.Perm(len(words) * 10) {
		if got >= *n {
			return
		}
		name := words[i%len(words)] + []string{"", "", "Fox", "Jay", "Wolf", "Bear", "Moth", "Owl", "Elk", "Cat"}[i/len(words)]
		url, isSlim, err := skinURL(name)
		if err != nil || url == "" || (isSlim && !*slim) {
			continue
		}
		resp, err := client.Get(url)
		if err != nil {
			continue
		}
		b, err := io.ReadAll(resp.Body)
		resp.Body.Close()
		if err != nil {
			continue
		}
		img, err := png.Decode(bytes.NewReader(b))
		if err != nil || img.Bounds().Dx() != 64 || img.Bounds().Dy() != 64 {
			continue // legacy 64x32 skins lack the 1.8 overlay layout
		}
		path := filepath.Join(*out, name+".png")
		if err := os.WriteFile(path, b, 0o644); err != nil {
			log.Fatal(err)
		}
		fmt.Println(path)
		got++
		time.Sleep(200 * time.Millisecond) // be polite to the API
	}
	log.Fatalf("only found %d skins", got)
}
