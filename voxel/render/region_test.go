package render

import "testing"

func TestFitGrowsLastPage(t *testing.T) {
	pg := &region{}
	pg.arena.grow(minPageVerts)
	pages := []*region{pg}
	if _, _, _, ok := fit(pages, minPageVerts, 0); !ok {
		t.Fatal("first fill")
	}
	got, off, toff, ok := fit(pages, 3*minPageVerts, 10)
	if !ok || got != pg || off != minPageVerts || toff != 4*minPageVerts || pg.arena.cap != 8*minPageVerts {
		t.Fatalf("grow: ok %v off %d toff %d cap %d", ok, off, toff, pg.arena.cap)
	}
	if _, _, _, ok := fit(pages, maxPageVerts, 0); ok || pg.arena.cap != maxPageVerts {
		t.Fatalf("over max: ok %v cap %d", ok, pg.arena.cap)
	}
}
