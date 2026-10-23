package ui

const bookTex = "textures/gui/book.png"

// Book image size (bookImageWidth, bookImageHeight).
const bookW, bookH = 192, 192

// BookScreen is GuiScreenBook: a written book read page by page, or a book
// and quill (Unsigned) written and signed. CustomNPC's GuiBook (TileBook
// blocks) is the same screen that does not pause the game (NoPause).
type BookScreen struct {
	// Pages is the "pages" tag; nil when the stack has none.
	Pages    []string
	Unsigned bool
	// Player is the editing player's name, shown as the author while
	// signing.
	Player string
	// Save is sendBookToServer: on Done, and on signing, when an unsigned
	// book was changed. pages lost their trailing empty pages; sign is
	// true when the book was signed with title.
	Save    func(pages []string, title string, sign bool)
	NoPause bool

	ctx                          *Context
	ready                        bool
	total, page, updates         int
	signing, changed             bool
	title                        string
	sign, done, finalize, cancel *Button
	next, prev                   *Button
	buttons                      []*Button // buttonList order
}

// PausesGame: GuiScreenBook pauses (GuiScreen.doesGuiPauseGame), GuiBook not.
func (b *BookScreen) PausesGame() bool { return !b.NoPause }

// setup is the constructor: a copy of the pages, at least one page.
func (b *BookScreen) setup() {
	b.ready = true
	if b.Pages != nil {
		b.Pages = append([]string(nil), b.Pages...)
	}
	if b.Pages == nil && b.Unsigned {
		b.Pages = []string{""}
	}
	b.total = max(len(b.Pages), 1)
}

// Init is initGui.
func (b *BookScreen) Init(ctx *Context) {
	if !b.ready {
		b.setup()
	}
	b.ctx = ctx
	y := 4 + bookH
	if b.Unsigned {
		b.sign = NewButton(ctx.W/2-100, y, 98, ctx.T("book.signButton"), func() { b.action(3) })
		b.done = NewButton(ctx.W/2+2, y, 98, ctx.T("gui.done"), func() { b.action(0) })
		b.finalize = NewButton(ctx.W/2-100, y, 98, ctx.T("book.finalizeButton"), func() { b.action(5) })
		b.cancel = NewButton(ctx.W/2+2, y, 98, ctx.T("gui.cancel"), func() { b.action(4) })
		b.buttons = []*Button{b.sign, b.done, b.finalize, b.cancel}
	} else {
		b.done = NewButton(ctx.W/2-100, y, 200, ctx.T("gui.done"), func() { b.action(0) })
		b.buttons = []*Button{b.done}
	}
	i := (ctx.W - bookW) / 2
	const b0 = 2
	b.next = &Button{X: i + 120, Y: b0 + 154, W: 23, H: 13, OnClick: func() { b.action(1) }}
	b.prev = &Button{X: i + 38, Y: b0 + 154, W: 23, H: 13, OnClick: func() { b.action(2) }}
	b.buttons = append(b.buttons, b.next, b.prev)
	b.updateButtons()
}

func (b *BookScreen) updateButtons() {
	b.next.Hidden = b.signing || !(b.page < b.total-1 || b.Unsigned)
	b.prev.Hidden = b.signing || b.page <= 0
	b.done.Hidden = b.Unsigned && b.signing
	if b.Unsigned {
		b.sign.Hidden = b.signing
		b.cancel.Hidden = !b.signing
		b.finalize.Hidden = !b.signing
		b.finalize.Disabled = len(trimSpace(b.title)) == 0
	}
}

// trimSpace is String.trim: chars <= ' ' off both ends.
func trimSpace(s string) string {
	i, j := 0, len(s)
	for i < j && s[i] <= ' ' {
		i++
	}
	for j > i && s[j-1] <= ' ' {
		j--
	}
	return s[i:j]
}

// close is displayGuiScreen(null).
func (b *BookScreen) close() { b.ctx.Screens.Back() }

// send is sendBookToServer.
func (b *BookScreen) send(sign bool) {
	if !b.Unsigned || !b.changed || b.Pages == nil {
		return
	}
	for len(b.Pages) > 1 && b.Pages[len(b.Pages)-1] == "" {
		b.Pages = b.Pages[:len(b.Pages)-1]
	}
	if b.Save != nil {
		title := ""
		if sign {
			title = trimSpace(b.title)
		}
		b.Save(append([]string(nil), b.Pages...), title, sign)
	}
}

// action is actionPerformed.
func (b *BookScreen) action(id int) {
	switch {
	case id == 0:
		b.close()
		b.send(false)
	case id == 3 && b.Unsigned:
		b.signing = true
	case id == 1:
		if b.page < b.total-1 {
			b.page++
		} else if b.Unsigned {
			b.addPage()
			if b.page < b.total-1 {
				b.page++
			}
		}
	case id == 2:
		if b.page > 0 {
			b.page--
		}
	case id == 5 && b.signing:
		b.send(true)
		b.close()
	case id == 4 && b.signing:
		b.signing = false
	}
	b.updateButtons()
}

// addPage is addNewPage: up to 50 pages.
func (b *BookScreen) addPage() {
	if b.Pages != nil && len(b.Pages) < 50 {
		b.Pages = append(b.Pages, "")
		b.total++
		b.changed = true
	}
}

func (b *BookScreen) Update() { b.updates++ }

// allowedChar is ChatAllowedCharacters.isAllowedCharacter.
func allowedChar(c rune) bool { return c != '§' && c >= ' ' && c != 127 }

// Input is mouseClicked (every button under the mouse, in list order) and
// keyTyped; Escape is left to the Manager (GuiScreen.keyTyped closes).
func (b *BookScreen) Input(ctx *Context, ev Event) bool {
	switch ev.Type {
	case MousePress:
		if ev.Button == 0 {
			for _, bt := range b.buttons {
				bt.Input(ctx, ev)
			}
		}
		return true
	case KeyPress, CharTyped:
		if ev.Type == KeyPress && ev.Key == KeyEscape {
			return false
		}
		if b.Unsigned {
			if b.signing {
				b.typeTitle(ev)
			} else {
				b.typeInBook(ctx, ev)
			}
		}
		return true
	}
	return false
}

func isEnter(ev Event) bool {
	return ev.Type == KeyPress && (ev.Key == KeyEnter || ev.Key == KeyKPEnter)
}

func isBackspace(ev Event) bool { return ev.Type == KeyPress && ev.Key == KeyBackspace }

// typeInBook is keyTypedInBook. ponytail: Ctrl+V (paste) is left out, the
// UI has no clipboard.
func (b *BookScreen) typeInBook(ctx *Context, ev Event) {
	switch {
	case isBackspace(ev):
		if rs := []rune(b.pageText()); len(rs) > 0 {
			b.setPage(string(rs[:len(rs)-1]))
		}
	case isEnter(ev):
		b.insert(ctx, "\n")
	case ev.Type == CharTyped && allowedChar(ev.Char):
		b.insert(ctx, string(ev.Char))
	}
}

// typeTitle is func_146460_c: the title, up to 16 chars.
func (b *BookScreen) typeTitle(ev Event) {
	switch {
	case isBackspace(ev):
		if rs := []rune(b.title); len(rs) > 0 {
			b.title = string(rs[:len(rs)-1])
			b.updateButtons()
		}
	case isEnter(ev):
		if b.title != "" {
			b.send(true)
			b.close()
		}
	case ev.Type == CharTyped && len([]rune(b.title)) < 16 && allowedChar(ev.Char):
		b.title += string(ev.Char)
		b.updateButtons()
		b.changed = true
	}
}

// pageText is func_146456_p.
func (b *BookScreen) pageText() string {
	if b.page >= 0 && b.page < len(b.Pages) {
		return b.Pages[b.page]
	}
	return ""
}

// setPage is func_146457_a.
func (b *BookScreen) setPage(s string) {
	if b.page >= 0 && b.page < len(b.Pages) {
		b.Pages[b.page] = s
		b.changed = true
	}
}

// insert is func_146459_b: the text is added while the page with a cursor
// still wraps into 118 px of lines and stays under 256 chars.
func (b *BookScreen) insert(ctx *Context, s string) {
	s2 := b.pageText() + s
	if len(ctx.Font.Wrap(s2+"§0_", 118))*ctx.Font.Height <= 118 && len([]rune(s2)) < 256 {
		b.setPage(s2)
	}
}

// cursor is the blinking "_" of an unsigned book.
func (b *BookScreen) cursor() string {
	if b.updates/6%2 == 0 {
		return "§0_"
	}
	return "§7_"
}

// Draw is drawScreen: no background, the book image, the page or the sign
// view, then the buttons.
func (b *BookScreen) Draw(ctx *Context, mx, my int, partial float32) {
	c, f := ctx.Canvas, ctx.Font
	tex := ctx.Tex(bookTex)
	k := (ctx.W - bookW) / 2
	const b0 = 2
	DrawTexturedModalRect(c, tex, k, b0, 0, 0, bookW, bookH)
	if b.signing {
		s := b.title
		if b.Unsigned {
			s += b.cursor()
		}
		s1 := ctx.T("book.editTitle")
		f.Draw(c, s1, k+36+(116-f.Width(s1))/2, b0+16+16, 0, false)
		f.Draw(c, s, k+36+(116-f.Width(s))/2, b0+48, 0, false)
		s2 := ctx.T("book.byAuthor", b.Player)
		f.Draw(c, "§8"+s2, k+36+(116-f.Width(s2))/2, b0+48+10, 0, false)
		f.DrawSplit(c, ctx.T("book.finalizeWarning"), k+36, b0+80, 116, 0, false)
	} else {
		s := ctx.T("book.pageIndicator", b.page+1, b.total)
		s1 := b.pageText()
		if b.Unsigned {
			s1 += b.cursor()
		}
		f.Draw(c, s, k-f.Width(s)+bookW-44, b0+16, 0, false)
		f.DrawSplit(c, s1, k+36, b0+16+16, 116, 0, false)
	}
	for _, bt := range b.buttons {
		if bt == b.next || bt == b.prev {
			if !bt.Hidden { // NextPageButton.drawButton
				u, v := 0, 192
				if bt.Hovered(mx, my) {
					u += 23
				}
				if bt == b.prev {
					v += 13
				}
				DrawTexturedModalRect(c, tex, bt.X, bt.Y, u, v, 23, 13)
			}
			continue
		}
		bt.Draw(ctx, mx, my)
	}
}
