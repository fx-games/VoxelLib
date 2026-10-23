package ui

import (
	"archive/zip"
	"image"
	"image/color"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

// vanillaFS opens the Minecraft 1.7.10 client jar from the Prism library
// cache when it is there (skips otherwise).
func vanillaFS(t *testing.T) fs.FS {
	home, _ := os.UserHomeDir()
	jar := filepath.Join(home, "Library/Application Support/PrismLauncher/libraries/com/mojang/minecraft/1.7.10/minecraft-1.7.10-client.jar")
	zr, err := zip.OpenReader(jar)
	if err != nil {
		t.Skip("no Minecraft 1.7.10 jar:", err)
	}
	t.Cleanup(func() { zr.Close() })
	return zr
}

func TestVanillaWidths(t *testing.T) {
	f, err := LoadFont(vanillaFS(t), nil)
	if err != nil {
		t.Fatal(err)
	}
	want := map[rune]int{'i': 2, 'l': 3, 't': 4, 'I': 4, 'f': 5, 'k': 5, 'A': 6, 'W': 6, '@': 7, '~': 7, '.': 2, '!': 2, ' ': 4, '§': -1, 'ä': 6, 'Ü': 6}
	for r, w := range want {
		if g := f.CharWidth(r); g != w {
			t.Errorf("width %q = %d, want %d", r, g, w)
		}
	}
	if g := f.Width("Hello"); g != 24 {
		t.Errorf("Hello = %d, want 24", g)
	}
	// Bold adds one pixel per glyph; § codes are free.
	if g := f.Width("§lAB§rC"); g != 6+1+6+1+6 {
		t.Errorf("bold = %d", g)
	}
	// A character only on the unicode pages, measured by glyph_sizes.bin.
	if g := f.CharWidth('ア'); g <= 0 || g > 9 {
		t.Errorf("katakana width %d", g)
	}
}

func TestMeasureSheet(t *testing.T) {
	// 128x128 sheet: 'A' (index 65) filled 5 columns, 'i' 1 column.
	img := image.NewNRGBA(image.Rect(0, 0, 128, 128))
	fill := func(idx, cols int) {
		for x := 0; x < cols; x++ {
			img.Set(idx%16*8+x, idx/16*8+3, color.White)
		}
	}
	fill(65, 5)
	fill(105, 1)
	f := &Font{Height: 9, glyphWidth: make([]byte, 65536)}
	f.SetASCII(img)
	if f.CharWidth('A') != 6 || f.CharWidth('i') != 2 || f.CharWidth('B') != 1 {
		t.Fatal(f.CharWidth('A'), f.CharWidth('i'), f.CharWidth('B'))
	}
	// An HD sheet (2x) measures in 8-pixel units.
	hd := image.NewNRGBA(image.Rect(0, 0, 256, 256))
	for x := 0; x < 9; x++ { // 9 of 16 columns -> 4.5 -> 5 (+1)
		hd.Set(65%16*16+x, 65/16*16+2, color.White)
	}
	f.SetASCII(hd)
	if g := f.CharWidth('A'); g != 6 {
		t.Fatal("hd", g)
	}
}

func testFont(t *testing.T) *Font {
	// Every glyph 5 wide (+1 = 6), space 4.
	img := image.NewNRGBA(image.Rect(0, 0, 128, 128))
	for i := 0; i < 256; i++ {
		for x := 0; x < 5; x++ {
			img.Set(i%16*8+x, i/16*8, color.White)
		}
	}
	f := &Font{Height: 9, glyphWidth: make([]byte, 65536)}
	f.SetASCII(img)
	return f
}

func TestWrap(t *testing.T) {
	f := testFont(t)
	// "aaa bbb ccc": each word 18 px, space 4.
	got := f.Wrap("aaa bbb ccc", 40)
	if !reflect.DeepEqual(got, []string{"aaa bbb", "ccc"}) {
		t.Fatalf("%q", got)
	}
	// Formatting carries over.
	got = f.Wrap("§caaa bbb ccc", 30)
	if !reflect.DeepEqual(got, []string{"§caaa", "§cbbb", "§cccc"}) {
		t.Fatalf("%q", got)
	}
	// Hard newlines and over-long words.
	got = f.Wrap("aa\nbbbbbbbbbb", 30)
	if !reflect.DeepEqual(got, []string{"aa", "bbbbb", "bbbbb"}) {
		t.Fatalf("%q", got)
	}
	if s := f.TrimToWidth("abcdef", 20, false); s != "abc" {
		t.Fatal(s)
	}
	if s := f.TrimToWidth("abcdef", 20, true); s != "def" {
		t.Fatal(s)
	}
}

func TestDrawShadowAndColour(t *testing.T) {
	f := testFont(t)
	c := NewSoftCanvas(64, 16, 1)
	f.ascii = c.NewTexture(image.NewNRGBA(image.Rect(0, 0, 128, 128)))
	var quads []Quad
	rc := &recCanvas{SoftCanvas: c, quads: &quads}
	end := f.Draw(rc, "a§cb", 2, 2, 0xFFFFFF, true)
	if end != 3+12 { // max of shadow and text end, like drawString
		t.Fatal("end", end)
	}
	// shadow a, shadow b, a, b
	if len(quads) != 4 {
		t.Fatal(len(quads))
	}
	if quads[0].Top != 0xFF3F3F3F || quads[1].Top != 0xFF3F1515 || quads[2].Top != 0xFFFFFFFF || quads[3].Top != 0xFFFF5555 {
		t.Fatalf("%08x %08x %08x %08x", quads[0].Top, quads[1].Top, quads[2].Top, quads[3].Top)
	}
	if quads[0].X0 != 3 || quads[2].X0 != 2 || quads[3].X0 != 8 {
		t.Fatal(quads[0].X0, quads[2].X0, quads[3].X0)
	}
}

type recCanvas struct {
	*SoftCanvas
	quads *[]Quad
}

func (r *recCanvas) Quad(q Quad) { *r.quads = append(*r.quads, q); r.SoftCanvas.Quad(q) }

func TestScaledResolution(t *testing.T) {
	for _, c := range []struct{ w, h, gs, wantS, wantW int }{
		{1920, 1080, 0, 4, 480}, {1920, 1080, 2, 2, 960}, {854, 480, 0, 2, 427}, {640, 480, 3, 2, 320}, {639, 480, 0, 1, 639},
	} {
		w, _, s := ScaledResolution(c.w, c.h, c.gs, false)
		if s != c.wantS || w != c.wantW {
			t.Errorf("%dx%d gs %d: %d %d", c.w, c.h, c.gs, w, s)
		}
	}
	if _, _, s := ScaledResolution(1920, 1080, 3, true); s != 2 {
		t.Error("unicode", s)
	}
}

func TestJavaRandom(t *testing.T) {
	var r javaRandom
	r.SetSeed(42)
	// new Random(42).nextInt(10) x3 = 0, 3, 8
	got := []int32{r.NextInt(10), r.NextInt(10), r.NextInt(10)}
	if !reflect.DeepEqual(got, []int32{0, 3, 8}) {
		t.Fatal(got)
	}
}
