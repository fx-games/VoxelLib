package ui

import "testing"

// A disabled field moves its cursor but keeps its text (setEnabled).
func TestTextFieldDisabled(t *testing.T) {
	f := &TextField{Focused: true, Disabled: true}
	f.SetText("ab")
	for _, ev := range []Event{{Type: CharTyped, Char: 'x'}, {Type: KeyPress, Key: KeyBackspace}, {Type: KeyPress, Key: KeyLeft}, {Type: KeyPress, Key: KeyDelete}} {
		if !f.Input(nil, ev) {
			t.Fatalf("%+v not taken", ev)
		}
	}
	if f.Text != "ab" || f.cursor != 1 {
		t.Fatalf("text %q cursor %d", f.Text, f.cursor)
	}
	f.Disabled = false
	f.Input(nil, Event{Type: CharTyped, Char: 'x'})
	if f.Text != "axb" {
		t.Fatalf("enabled text %q", f.Text)
	}
}
