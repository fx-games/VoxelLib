package ui

import (
	"runtime"
	"testing"
)

// TestPaste: Cmd+V (Ctrl+V off macOS) writes the clipboard's allowed
// characters at the cursor, cut to the max length; a disabled field and a
// plain V paste nothing.
func TestPaste(t *testing.T) {
	mod := ModControl
	if runtime.GOOS == "darwin" {
		mod = ModSuper
	}
	ctx := &Context{Clipboard: func() string { return "x§y\tz1234" }}
	f := &TextField{Focused: true, MaxLen: 6}
	f.SetText("ab")
	f.cursor = 1
	if !f.Input(ctx, Event{Type: KeyPress, Key: KeyV, Mods: mod}) || f.Text != "axyz1b" || f.cursor != 5 {
		t.Fatalf("paste: %q cursor %d", f.Text, f.cursor)
	}
	f.Input(ctx, Event{Type: KeyPress, Key: KeyV})
	f.Disabled = true
	f.Input(ctx, Event{Type: KeyPress, Key: KeyV, Mods: mod})
	if f.Text != "axyz1b" {
		t.Fatalf("pasted without the combo or disabled: %q", f.Text)
	}
	if (&Context{}).ClipboardText() != "" {
		t.Fatal("no clipboard")
	}
}
