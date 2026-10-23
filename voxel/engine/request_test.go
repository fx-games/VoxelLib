package engine

import (
	"testing"

	"github.com/memmaker/voxellib/voxel"
)

func TestRequestChunksRetriesWhenFull(t *testing.T) {
	e := &Engine{loaded: map[voxel.ChunkPos]bool{}, pending: map[voxel.ChunkPos]bool{},
		loadReq: make(chan voxel.ChunkPos, 5)}
	if e.requestChunks(1) {
		t.Fatal("9 chunks fit a queue of 5")
	}
	if p := <-e.loadReq; p != (voxel.ChunkPos{}) {
		t.Errorf("first request %v, want the centre", p)
	}
	for len(e.loadReq) > 0 {
		<-e.loadReq
	}
	if !e.requestChunks(1) || len(e.pending) != 9 {
		t.Errorf("second pass: %d pending, want 9", len(e.pending))
	}
}
