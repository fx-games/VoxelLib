package ui

import "testing"

// TestChatScreenStay: GuiSleepMP's Enter sends the trimmed line, adds it
// to the sent messages and clears the line, the screen stays (the chat
// open); in a chat opened later up and down walk the sent messages back
// to the typed buffer.
func TestChatScreenStay(t *testing.T) {
	ctx, _ := testContext(t)
	h := &HUD{}
	var got []string
	cs := &ChatScreen{HUD: h, Stay: true, OnSend: func(s string) { got = append(got, s) }}
	ctx.Screens.Open(cs)
	typ := func(s string) {
		for _, r := range s {
			ctx.Screens.Input(Event{Type: CharTyped, Char: r})
		}
	}
	typ("  hello ")
	ctx.Screens.Input(Event{Type: KeyPress, Key: KeyEnter})
	ctx.Screens.Input(Event{Type: KeyPress, Key: KeyEnter}) // empty: nothing sent
	if len(got) != 1 || got[0] != "hello" || cs.field.Text != "" || ctx.Screens.Top() != cs || !chatOpen(ctx) {
		t.Fatalf("sent %q, line %q", got, cs.field.Text)
	}
	ctx.Screens.Back()
	cs = &ChatScreen{HUD: h} // initGui puts the history cursor after the sent messages
	ctx.Screens.Open(cs)
	typ("draft")
	ctx.Screens.Input(Event{Type: KeyPress, Key: KeyUp})
	if cs.field.Text != "hello" {
		t.Fatalf("up: %q", cs.field.Text)
	}
	ctx.Screens.Input(Event{Type: KeyPress, Key: KeyDown})
	if cs.field.Text != "draft" {
		t.Fatalf("down: %q", cs.field.Text)
	}
	ctx.Screens.Back()
	if chatOpen(ctx) {
		t.Fatal("chat open with no screen")
	}
}
