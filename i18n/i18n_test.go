package i18n

import (
	"strings"
	"testing"
	"testing/fstest"
)

func TestCatalog(t *testing.T) {
	c := New()
	c.Load("en_US", strings.NewReader("\uFEFFlanguage.name=English\nlanguage.region=US\n# comment\nmenu.quit=Quit Game\ngui.done=Done\nkey.a=Hit %d times, %.1f%%\nbad line\nx==y=z\n"))
	c.Load("de_DE", strings.NewReader("language.name=Deutsch\nlanguage.region=Deutschland\nmenu.quit=Spiel beenden\r\norder=%2$s vor %1$s\n"))
	if g := c.T("menu.quit"); g != "Quit Game" {
		t.Fatal(g)
	}
	c.SetLanguage("de_DE")
	if g := c.T("menu.quit"); g != "Spiel beenden" {
		t.Fatal(g)
	}
	if g := c.T("gui.done"); g != "Done" {
		t.Fatal("fallback", g)
	}
	if g := c.T("missing.key", 1); g != "missing.key" {
		t.Fatal(g)
	}
	if g := c.T("key.a", 3, 2.5); g != "Hit 3 times, 2.5%" {
		t.Fatal(g)
	}
	if g := c.T("order", "A", "B"); g != "B vor A" {
		t.Fatal(g)
	}
	if g := c.T("x"); g != "=y=z" {
		t.Fatal(g)
	}
	if g := c.Name("de_DE"); g != "Deutsch (Deutschland)" {
		t.Fatal(g)
	}
	if g := Format("%s %s", 1); !strings.HasPrefix(g, "Format error") {
		t.Fatal(g)
	}
	if g := Format("%s", 2.0); g != "2.0" {
		t.Fatal(g)
	}
}

func TestLoadFSPriority(t *testing.T) {
	low := fstest.MapFS{"assets/minecraft/lang/en_US.lang": {Data: []byte("a=low\nb=low\n")}}
	high := fstest.MapFS{"assets/mod/lang/en_US.lang": {Data: []byte("a=high\n")}}
	c := New()
	if err := c.LoadFS(low, high); err != nil {
		t.Fatal(err)
	}
	if c.T("a") != "high" || c.T("b") != "low" {
		t.Fatal(c.T("a"), c.T("b"))
	}
}
