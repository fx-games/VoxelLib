// Command bake writes a client jar's blockstates and flattened block models
// into a pack folder (respack.Bake): the first step of tools/shapegen/run.sh.
package main

import (
	"fmt"
	"log"
	"os"

	"github.com/memmaker/voxellib/voxel/respack"
)

func main() {
	if len(os.Args) != 3 {
		log.Fatal("usage: bake <client.jar> <dst folder>")
	}
	p, err := respack.Open(os.Args[1])
	if err != nil {
		log.Fatal(err)
	}
	defer p.Close()
	n, err := p.Bake(os.Args[2])
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("baked %d blockstates\n", n)
}
