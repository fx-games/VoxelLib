package render

import (
	"testing"

	"github.com/memmaker/voxellib/voxel"
)

func TestWeather(t *testing.T) {
	reg := voxel.NewRegistry()
	w := voxel.NewWorld(reg, 0, 64)
	stone := reg.Default("stone")
	c := w.NewChunk(voxel.ChunkPos{})
	for x := range 16 {
		for z := range 16 {
			for y := range 4 {
				c.SetRaw(x, y, z, stone)
			}
		}
	}
	for y := range 10 {
		c.SetRaw(3, y, 0, stone) // pillar above the strips: no column
	}
	w.AddChunk(c)
	r := &Renderer{world: w, Opt: Options{RenderDistance: 8}}
	wt := &Weather{Strength: 1, Feet: [3]float64{0.5, 4, 0.5}, Radius: 5,
		Textures: [PrecipCount]string{PrecipRain: "rain", PrecipSnow: "snow", PrecipAsh: "ash"},
		Column: func(x, y, z int) Precip {
			switch {
			case x < 0 || z < 0:
				return PrecipNone
			case x == 0:
				return PrecipSnow
			case x == 1:
				return PrecipAsh
			}
			return PrecipRain
		}}
	cam := &Camera{X: 0.5, Y: 5.62, Z: 0.5}
	out := r.weather(wt, cam, nil)
	n := 0
	for _, d := range out {
		n += len(d.Verts) / 4
	}
	if n != 6*6-2 {
		t.Fatalf("%d strips, want 34", n)
	}
	// Row z=0: ash at x=1, rain at x=2,4,5. Row z=1: snow at x=0 then ash
	// at x=1 share one binding, so ash falls with the snow texture as in
	// LOTRWeatherRenderer.
	if out[0].Texture != "ash" || out[1].Texture != "rain" || out[2].Texture != "snow" || len(out[2].Verts) != 8 {
		t.Fatalf("draw runs %q %q %q (%d verts)", out[0].Texture, out[1].Texture, out[2].Texture, len(out[2].Verts))
	}
	// The x=2 strip faces the viewer (across z) from y 4 to 9.
	v := out[1].Verts[:4]
	if v[0].Pos != [3]float32{2, 4 - 5.62, 0.5 - 0.5 - 0.5} || v[2].Pos != [3]float32{2, 9 - 5.62, 0.5 + 0.5 - 0.5} {
		t.Errorf("rain strip %v %v", v[0].Pos, v[2].Pos)
	}
	if dv := v[2].UV[1] - v[0].UV[1]; dv != 1.25 {
		t.Errorf("v spans %v, want 5/4", dv)
	}
}

func TestLightmapAt(t *testing.T) {
	var lm Lightmap
	for s := range 16 {
		for b := range 16 {
			lm[s][b][0] = float32(s*16 + b)
		}
	}
	if got := lm.At(1.5, 2.25)[0]; got != 26.25 {
		t.Errorf("At = %v", got)
	}
	if got := lm.At(20, 15)[0]; got != 255 {
		t.Errorf("clamped At = %v", got)
	}
}

// TestBlockBoost: a fractional block light boost is filtered between the
// lightmap's levels and capped at 15.
func TestBlockBoost(t *testing.T) {
	var r Renderer
	for s := range 16 {
		for b := range 16 {
			r.light[s][b][0] = float32(b * 10)
		}
	}
	for _, c := range []struct {
		block uint8
		want  float32
	}{{3, 105}, {10, 150}, {0, 75}} {
		if got := r.modelLight(drawParams{sky: 4, block: c.block, boost: 7.5})[0]; got != c.want {
			t.Errorf("block %d: %v, want %v", c.block, got, c.want)
		}
	}
	if got := r.modelLight(drawParams{sky: 4, block: 3})[0]; got != 30 {
		t.Errorf("no boost: %v", got)
	}
}

// TestGlintPasses: Glint is one pass, Glints adds to it.
func TestGlintPasses(t *testing.T) {
	for _, c := range []struct {
		glint  bool
		glints int
		want   int
	}{{false, 0, 0}, {true, 0, 1}, {false, 4, 4}, {true, 4, 5}} {
		ir := ItemRenderer{glint: c.glint, glints: c.glints}
		if got := ir.glintPasses(); got != c.want {
			t.Errorf("%+v: %d", c, got)
		}
	}
}
