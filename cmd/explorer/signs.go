package main

// Sign text: voxel/signtext draws each sign's lines on its board.

import (
	"os"

	"github.com/memmaker/voxellib/voxel"
	"github.com/memmaker/voxellib/voxel/render"
	"github.com/memmaker/voxellib/voxel/signtext"
)

// loadSignFont opens a pixel TTF at its native 8px size ("" = built in).
func (g *game) loadSignFont(path string) error {
	var b []byte
	if path != "" {
		var err error
		if b, err = os.ReadFile(path); err != nil {
			return err
		}
	}
	s, err := signtext.New(g.Models, b)
	if err != nil {
		return err
	}
	if m := g.Config.Engine.Map; m != nil { // a map's keys hold no facing
		s.Block = func(_ *voxel.World, id voxel.StateID) (string, map[string]string) { return m.JavaState(id) }
	}
	g.signs = s
	return nil
}

// drawSigns draws sign text within signtext.Distance of the camera.
func (g *game) drawSigns(cam *render.Camera) {
	if g.signs != nil {
		g.signs.Draw(cam, g.Eng.World, g.Light)
	}
}
