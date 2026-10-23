package ui

import (
	"fmt"
	"image"
	_ "image/png"
	"io"
	"io/fs"
	"strings"
	"unicode"
)

// Font is Minecraft 1.7.10's FontRenderer: the ascii.png sheet (16x16 glyphs
// of any size, widths measured from the image like readFontTexture) plus the
// unicode pages (unicode_page_XX.png, widths from glyph_sizes.bin) for
// characters the sheet lacks, § colour and format codes, shadows, wrapping
// and width measurement, all with Minecraft's integer rules.
type Font struct {
	// Unicode draws everything from the unicode pages (forceUnicodeFont).
	Unicode bool
	// Height is FONT_HEIGHT (9).
	Height int

	charWidth  [256]int
	glyphWidth []byte // 65536 entries: high nibble start, low nibble end column
	ascii      Texture
	pages      [256]Texture
	pageTried  [256]bool
	fsys       fs.FS
	newTex     func(image.Image) Texture
	rand       javaRandom

	// render state (renderStringAtPos)
	posX, posY                              float32
	ox, oy, scale                           float32
	red, green, blue, alpha                 float32
	color                                   uint32
	random, bold, strike, underline, italic bool
	canvas                                  Canvas
}

// asciiChars maps the cells of ascii.png (FontRenderer's index string).
const asciiChars = "ÀÁÂÈÊËÍÓÔÕÚßãõğİıŒœŞşŴŵžȇ\x00\x00\x00\x00\x00\x00\x00 !\"#$%&'()*+,-./0123456789:;<=>?@ABCDEFGHIJKLMNOPQRSTUVWXYZ[\\]^_`abcdefghijklmnopqrstuvwxyz{|}~\x00ÇüéâäàåçêëèïîìÄÅÉæÆôöòûùÿÖÜø£Ø×ƒáíóúñÑªº¿®¬½¼¡«»░▒▓│┤╡╢╖╕╣║╗╝╜╛┐└┴┬├─┼╞╟╚╔╩╦╠═╬╧╨╤╥╙╘╒╓╫╪┘┌█▄▌▐▀αβΓπΣσμτΦΘΩδ∞∅∈∩≡±≥≤⌠⌡÷≈°∙·√ⁿ²■\x00"

var asciiIndex = func() map[rune]int {
	m := map[rune]int{}
	for i, r := range []rune(asciiChars) {
		if _, ok := m[r]; !ok {
			m[r] = i
		}
	}
	return m
}()

func asciiIdx(r rune) int {
	if i, ok := asciiIndex[r]; ok {
		return i
	}
	return -1
}

// colorCodes are the 16 § colours and their shadow versions.
var colorCodes = func() [32]uint32 {
	var cc [32]uint32
	for i := 0; i < 32; i++ {
		j := (i >> 3 & 1) * 85
		k := (i>>2&1)*170 + j
		l := (i>>1&1)*170 + j
		m := (i&1)*170 + j
		if i == 6 {
			k += 85
		}
		if i >= 16 {
			k, l, m = k/4, l/4, m/4
		}
		cc[i] = uint32(k&255)<<16 | uint32(l&255)<<8 | uint32(m&255)
	}
	return cc
}()

// ColorCode returns the RGB of § colour code i (0..15).
func ColorCode(i int) uint32 { return colorCodes[i] }

// Font asset paths in a resource pack.
const (
	FontASCII      = "assets/minecraft/textures/font/ascii.png"
	FontGlyphSizes = "assets/minecraft/font/glyph_sizes.bin"
	fontPage       = "assets/minecraft/textures/font/unicode_page_%02x.png"
)

// LoadFont reads ascii.png and glyph_sizes.bin from fsys; unicode pages are
// loaded on first use. newTex uploads images (Canvas.NewTexture).
func LoadFont(fsys fs.FS, newTex func(image.Image) Texture) (*Font, error) {
	f := &Font{Height: 9, fsys: fsys, newTex: newTex}
	img, err := loadImage(fsys, FontASCII)
	if err != nil {
		return nil, err
	}
	f.SetASCII(img)
	if b, err := fs.ReadFile(fsys, FontGlyphSizes); err == nil {
		f.glyphWidth = b
	}
	if len(f.glyphWidth) < 65536 {
		f.glyphWidth = append(f.glyphWidth, make([]byte, 65536-len(f.glyphWidth))...)
	}
	f.rand.SetSeed(1)
	return f, nil
}

func loadImage(fsys fs.FS, name string) (image.Image, error) {
	r, err := fsys.Open(name)
	if err != nil {
		return nil, err
	}
	defer r.Close()
	img, _, err := image.Decode(r.(io.Reader))
	if err != nil {
		return nil, fmt.Errorf("%s: %w", name, err)
	}
	return img, nil
}

// SetASCII sets the glyph sheet and measures its widths (readFontTexture).
func (f *Font) SetASCII(img image.Image) {
	b := img.Bounds()
	w, h := b.Dx(), b.Dy()
	ch := h / 16
	cw := w / 16
	scale := float32(8) / float32(cw)
	for i := 0; i < 256; i++ {
		col, row := i%16, i/16
		l := cw - 1
		for ; l >= 0; l-- {
			empty := true
			for y := 0; y < ch && empty; y++ {
				// Minecraft offsets rows by the cell width (square cells).
				_, _, _, a := img.At(b.Min.X+col*cw+l, b.Min.Y+row*cw+y).RGBA()
				if a>>8 != 0 {
					empty = false
				}
			}
			if !empty {
				break
			}
		}
		l++
		f.charWidth[i] = int(0.5+float64(float32(l)*scale)) + 1
	}
	if f.newTex != nil {
		f.ascii = f.newTex(img)
	}
}

func (f *Font) page(p int) Texture {
	if !f.pageTried[p] {
		f.pageTried[p] = true
		if img, err := loadImage(f.fsys, fmt.Sprintf(fontPage, p)); err == nil && f.newTex != nil {
			f.pages[p] = f.newTex(img)
		}
	}
	return f.pages[p]
}

// CharWidth is getCharWidth: -1 for §, 4 for space.
func (f *Font) CharWidth(c rune) int {
	if c == '§' {
		return -1
	}
	if c == ' ' {
		return 4
	}
	if i := asciiIdx(c); c > 0 && i != -1 && !f.Unicode {
		return f.charWidth[i]
	}
	if c < 65536 && f.glyphWidth[c] != 0 {
		j := int(f.glyphWidth[c] >> 4)
		k := int(f.glyphWidth[c] & 15)
		if k > 7 {
			k, j = 15, 0
		}
		k++
		return (k-j)/2 + 1
	}
	return 0
}

// Width is getStringWidth.
func (f *Font) Width(s string) int {
	rs := []rune(s)
	w, bold := 0, false
	for i := 0; i < len(rs); i++ {
		k := f.CharWidth(rs[i])
		if k < 0 && i < len(rs)-1 {
			i++
			c := rs[i]
			if c == 'l' || c == 'L' {
				bold = true
			} else if c == 'r' || c == 'R' {
				bold = false
			}
			k = 0
		}
		w += k
		if bold && k > 0 {
			w++
		}
	}
	return w
}

// Draw is drawString: s at x,y, with a shadow one pixel down-right when
// shadow is set. color is 0xAARRGGBB (alpha 0 means opaque). It returns the
// x after the text.
func (f *Font) Draw(c Canvas, s string, x, y int, color uint32, shadow bool) int {
	return f.DrawScaled(c, s, 0, 0, 1, float32(x), float32(y), color, shadow)
}

// DrawCentered is drawCenteredString (with shadow).
func (f *Font) DrawCentered(c Canvas, s string, x, y int, color uint32) {
	f.Draw(c, s, x-f.Width(s)/2, y, color, true)
}

// DrawScaled draws s at local x,y with the transform screen = o + local*scale
// (glTranslate/glScale around the text, as titles do).
func (f *Font) DrawScaled(c Canvas, s string, ox, oy, scale, x, y float32, color uint32, shadow bool) int {
	f.canvas, f.ox, f.oy, f.scale = c, ox, oy, scale
	f.random, f.bold, f.strike, f.underline, f.italic = false, false, false, false, false
	if shadow {
		e := f.render(s, x+1, y+1, color, true)
		return max(e, f.render(s, x, y, color, false))
	}
	return f.render(s, x, y, color, false)
}

func (f *Font) render(s string, x, y float32, color uint32, shadow bool) int {
	if color&0xFC000000 == 0 {
		color |= 0xFF000000
	}
	if shadow {
		color = (color&0xFCFCFC)>>2 | color&0xFF000000
	}
	f.red, f.green, f.blue = float32(color>>16&255)/255, float32(color>>8&255)/255, float32(color&255)/255
	f.alpha = float32(color>>24&255) / 255
	f.color = color
	f.posX, f.posY = x, y
	f.renderAt([]rune(s), shadow)
	return int(f.posX)
}

func (f *Font) setColor(r, g, b, a float32) {
	f.color = uint32(a*255+0.5)<<24 | uint32(r*255+0.5)<<16 | uint32(g*255+0.5)<<8 | uint32(b*255+0.5)
}

func (f *Font) renderAt(rs []rune, shadow bool) {
	for i := 0; i < len(rs); i++ {
		c := rs[i]
		if c == '§' && i+1 < len(rs) {
			j := strings.IndexRune("0123456789abcdefklmnor", unicode.ToLower(rs[i+1]))
			switch {
			case j < 16:
				f.random, f.bold, f.strike, f.underline, f.italic = false, false, false, false, false
				if j < 0 || j > 15 {
					j = 15
				}
				if shadow {
					j += 16
				}
				k := colorCodes[j]
				f.setColor(float32(k>>16)/255, float32(k>>8&255)/255, float32(k&255)/255, f.alpha)
			case j == 16:
				f.random = true
			case j == 17:
				f.bold = true
			case j == 18:
				f.strike = true
			case j == 19:
				f.underline = true
			case j == 20:
				f.italic = true
			case j == 21:
				f.random, f.bold, f.strike, f.underline, f.italic = false, false, false, false, false
				f.setColor(f.red, f.green, f.blue, f.alpha)
			}
			i++
			continue
		}
		j := asciiIdx(c)
		if f.random && j != -1 {
			var k int
			for {
				k = int(f.rand.NextInt(256))
				if f.charWidth[j] == f.charWidth[k] {
					break
				}
			}
			j = k
		}
		off := float32(1)
		if f.Unicode {
			off = 0.5
		}
		shift := (c == 0 || j == -1 || f.Unicode) && shadow
		if shift {
			f.posX -= off
			f.posY -= off
		}
		w := f.renderChar(j, c, f.italic)
		if shift {
			f.posX += off
			f.posY += off
		}
		if f.bold {
			f.posX += off
			if shift {
				f.posX -= off
				f.posY -= off
			}
			f.renderChar(j, c, f.italic)
			f.posX -= off
			if shift {
				f.posX += off
				f.posY += off
			}
			w++
		}
		if f.strike {
			f.rect(f.posX, f.posY+float32(f.Height/2)-1, f.posX+w, f.posY+float32(f.Height/2))
		}
		if f.underline {
			f.rect(f.posX-1, f.posY+float32(f.Height)-1, f.posX+w, f.posY+float32(f.Height))
		}
		f.posX += float32(int(w))
	}
}

func (f *Font) rect(x0, y0, x1, y1 float32) {
	f.quad(nil, x0, y0, x1, y1, 0, 0, 0, 0, 0)
}

func (f *Font) quad(t Texture, x0, y0, x1, y1, u0, v0, u1, v1, skew float32) {
	if f.canvas == nil {
		return
	}
	s := f.scale
	f.canvas.Quad(Quad{Tex: t, X0: f.ox + x0*s, Y0: f.oy + y0*s, X1: f.ox + x1*s, Y1: f.oy + y1*s,
		U0: u0, V0: v0, U1: u1, V1: v1, Skew: skew * s, Top: f.color, Bottom: f.color})
}

func (f *Font) renderChar(j int, c rune, italic bool) float32 {
	if c == ' ' {
		return 4
	}
	if j != -1 && !f.Unicode {
		return f.renderDefault(j, italic)
	}
	return f.renderUnicode(c, italic)
}

func (f *Font) renderDefault(j int, italic bool) float32 {
	u := float32(j % 16 * 8)
	v := float32(j / 16 * 8)
	sk := float32(0)
	if italic {
		sk = 1
	}
	w := float32(f.charWidth[j]) - 0.01
	f.quad(f.ascii, f.posX, f.posY, f.posX+w-1, f.posY+7.99, u/128, v/128, (u+w-1)/128, (v+7.99)/128, sk)
	return float32(f.charWidth[j])
}

func (f *Font) renderUnicode(c rune, italic bool) float32 {
	if c >= 65536 || f.glyphWidth[c] == 0 {
		return 0
	}
	t := f.page(int(c / 256))
	start := float32(f.glyphWidth[c] >> 4)
	end := float32(f.glyphWidth[c]&15) + 1
	u := float32(c%16*16) + start
	v := float32((c & 255) / 16 * 16)
	w := end - start - 0.02
	sk := float32(0)
	if italic {
		sk = 1
	}
	if t != nil {
		f.quad(t, f.posX, f.posY, f.posX+w/2, f.posY+7.99, u/256, v/256, (u+w)/256, (v+15.98)/256, sk)
	}
	return (end-start)/2 + 1
}

// TrimToWidth is trimStringToWidth: the longest prefix (or suffix when
// reverse) of s no wider than width.
func (f *Font) TrimToWidth(s string, width int, reverse bool) string {
	rs := []rune(s)
	var out []rune
	j := 0
	i, step := 0, 1
	if reverse {
		i, step = len(rs)-1, -1
	}
	code, bold := false, false
	for ; i >= 0 && i < len(rs) && j < width; i += step {
		c := rs[i]
		k := f.CharWidth(c)
		if code {
			code = false
			if c == 'l' || c == 'L' {
				bold = true
			} else if c == 'r' || c == 'R' {
				bold = false
			}
		} else if k < 0 {
			code = true
		} else {
			j += k
			if bold {
				j++
			}
		}
		if j > width {
			break
		}
		if reverse {
			out = append([]rune{c}, out...)
		} else {
			out = append(out, c)
		}
	}
	return string(out)
}

func isFormatColor(c rune) bool {
	return c >= '0' && c <= '9' || c >= 'a' && c <= 'f' || c >= 'A' && c <= 'F'
}

func isFormatSpecial(c rune) bool {
	return c >= 'k' && c <= 'o' || c >= 'K' && c <= 'O' || c == 'r' || c == 'R'
}

// FormatFromString is getFormatFromString: the § codes in effect at the
// end of s, to carry them over a line break.
func FormatFromString(s string) string {
	rs := []rune(s)
	out := ""
	for i := 0; i < len(rs)-1; i++ {
		if rs[i] != '§' {
			continue
		}
		c := rs[i+1]
		if isFormatColor(c) {
			out = "§" + string(c)
		} else if isFormatSpecial(c) {
			out += "§" + string(c)
		}
	}
	return out
}

// StripFormatting removes § codes.
func StripFormatting(s string) string {
	rs := []rune(s)
	var out []rune
	for i := 0; i < len(rs); i++ {
		if rs[i] == '§' && i+1 < len(rs) {
			i++
			continue
		}
		out = append(out, rs[i])
	}
	return string(out)
}

// sizeToWidth is sizeStringToWidth: how many runes of rs fit in width,
// breaking at the last space when the line overflows.
func (f *Font) sizeToWidth(rs []rune, width int) int {
	n := len(rs)
	k, l, space := 0, 0, -1
	bold := false
	for ; l < n; l++ {
		c := rs[l]
		switch c {
		case '\n':
			l--
		case '§':
			if l < n-1 {
				l++
				c1 := rs[l]
				if c1 == 'l' || c1 == 'L' {
					bold = true
				} else if c1 == 'r' || c1 == 'R' || isFormatColor(c1) {
					bold = false
				}
			}
		case ' ':
			space = l
			fallthrough
		default:
			k += f.CharWidth(c)
			if bold {
				k++
			}
		}
		if c == '\n' {
			l++
			space = l
			break
		}
		if k > width {
			break
		}
	}
	if l != n && space != -1 && space < l {
		return space
	}
	return l
}

// Wrap is listFormattedStringToWidth: s broken into lines no wider than
// width, at spaces where possible, with § formatting carried to each line.
func (f *Font) Wrap(s string, width int) []string {
	return strings.Split(f.wrap([]rune(s), width), "\n")
}

func (f *Font) wrap(rs []rune, width int) string {
	var b strings.Builder
	for {
		j := f.sizeToWidth(rs, width)
		if len(rs) <= j {
			b.WriteString(string(rs))
			return b.String()
		}
		head := string(rs[:j])
		skip := 0
		if c := rs[j]; c == ' ' || c == '\n' {
			skip = 1
		}
		b.WriteString(head)
		b.WriteByte('\n')
		rs = append([]rune(FormatFromString(head)), rs[j+skip:]...)
	}
}

// DrawSplit is drawSplitString: s wrapped to width, one line every Height.
func (f *Font) DrawSplit(c Canvas, s string, x, y, width int, color uint32, shadow bool) {
	for strings.HasSuffix(s, "\n") {
		s = s[:len(s)-1]
	}
	for _, line := range f.Wrap(s, width) {
		f.Draw(c, line, x, y, color, shadow)
		y += f.Height
	}
}
