package app

import "testing"

type pauseScreen struct{ pauses bool }

func (pauseScreen) Update()                {}
func (pauseScreen) Draw(int, int)          {}
func (pauseScreen) HandleInput(Event) bool { return true }
func (s pauseScreen) PausesGame() bool     { return s.pauses }

type plainScreen struct{}

func (plainScreen) Update()                {}
func (plainScreen) Draw(int, int)          {}
func (plainScreen) HandleInput(Event) bool { return true }

// TestPaused: isGamePaused is the top screen's doesGuiPauseGame; screens
// without PausesGame pause.
func TestPaused(t *testing.T) {
	a := &App{}
	if a.Paused() || a.ScreenOpen() {
		t.Fatal("paused without a screen")
	}
	a.screens = []Screen{pauseScreen{false}}
	if a.Paused() || !a.ScreenOpen() {
		t.Error("a non-pausing screen paused")
	}
	a.screens = append(a.screens, plainScreen{})
	if !a.Paused() {
		t.Error("a plain screen on top did not pause")
	}
	a.screens = []Screen{plainScreen{}, pauseScreen{false}}
	if a.Paused() {
		t.Error("the top screen decides")
	}
}
