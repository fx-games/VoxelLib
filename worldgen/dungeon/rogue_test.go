package dungeon

import (
	"math/rand"
	"testing"
)

func TestRogueGenerate(t *testing.T) {
	m := NewRogueGenerator(rand.New(rand.NewSource(1)), 60, 30).Generate()
	if m == nil {
		t.Fatal("nil map")
	}
}
