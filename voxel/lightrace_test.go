package voxel

import (
	"sync"
	"testing"
)

// TestLightPassRace: a light pass writes nibbles under the read lock while
// readers read them (go test -race).
func TestLightPassRace(t *testing.T) {
	w, _ := flatWorld(t)
	p := ChunkPos{0, 0}
	var wg sync.WaitGroup
	done := make(chan struct{})
	wg.Add(1)
	go func() {
		defer wg.Done()
		for {
			select {
			case <-done:
				return
			default:
				w.Light(5, 64, 5)
			}
		}
	}()
	for i := 0; i < 20; i++ {
		w.Lock()
		w.chunks[p].Lit.Store(false)
		w.Unlock()
		w.TryLightChunk(p)
	}
	close(done)
	wg.Wait()
	if s, _ := w.Light(5, 64, 5); s != 15 {
		t.Fatalf("sky %d", s)
	}
}

// TestLightPassLitRace: Renderable reads Lit while a pass sets it.
func TestLightPassLitRace(t *testing.T) {
	w, _ := flatWorld(t)
	p := ChunkPos{0, 0}
	done := make(chan struct{})
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		for {
			select {
			case <-done:
				return
			default:
				w.Renderable(p)
			}
		}
	}()
	for i := 0; i < 20; i++ {
		w.Lock()
		w.chunks[p].Lit.Store(false)
		w.Unlock()
		w.TryLightChunk(p)
	}
	close(done)
	wg.Wait()
}
