package engine

import (
	"testing"
	"time"

	"github.com/memmaker/voxellib/voxel"
	"github.com/memmaker/voxellib/voxel/mesh"
)

// A block changed off the render thread (World.SetBlock) next to the viewer
// is handed to the mesh workers in the next frame despite a backlog, and one
// frame later again if its section was already being meshed.
func TestDeferredRemeshLatency(t *testing.T) {
	reg := voxel.NewRegistry()
	reg.Register(&voxel.BlockDef{Name: "test:stone", Shape: voxel.ShapeCube})
	stone := reg.Default("test:stone")
	w := voxel.NewWorld(reg, -64, 384)
	e := &Engine{World: w, cfg: Config{RenderDistance: 3, UploadBudget: time.Second},
		lit: map[voxel.ChunkPos]bool{}, loaded: map[voxel.ChunkPos]bool{},
		dirty: map[voxel.SectionPos]bool{}, inFlight: map[voxel.SectionPos]bool{}, redo: map[voxel.SectionPos]bool{},
		staleMesh: map[voxel.SectionPos]bool{},
		meshJobs:  make(chan voxel.SectionPos, 2), meshRes: make(chan *mesh.Mesh, 4)}
	for x := -4; x <= 4; x++ {
		for z := -4; z <= 4; z++ {
			p := voxel.ChunkPos{X: x, Z: z}
			w.AddChunk(w.NewChunk(p))
			e.lit[p] = true
		}
	}
	e.viewer = [3]float64{8, 70, 8}
	// Update's order: take the world's marks, upload results, dispatch.
	frame := func() {
		for _, p := range w.TakeDirty() {
			e.dirty[p] = true
		}
		e.uploadResults()
		e.dispatch()
	}
	jobs := func() map[voxel.SectionPos]bool {
		m := map[voxel.SectionPos]bool{}
		for len(e.meshJobs) > 0 {
			m[<-e.meshJobs] = true
		}
		return m
	}
	for x := -3; x <= 3; x++ { // a far backlog
		for y := -4; y < 20; y++ {
			e.dirty[voxel.SectionPos{X: x, Y: y, Z: 3}] = true
		}
	}
	near := voxel.SectionPos{X: 0, Y: 4, Z: 0}
	w.SetBlock(8, 70, 8, stone)
	frame()
	if !jobs()[near] {
		t.Fatal("the edited section was not dispatched in the next frame")
	}

	e.inFlight[near] = true // being meshed when the next change lands
	w.SetBlock(8, 71, 8, stone)
	frame()
	if jobs()[near] || !e.redo[near] {
		t.Fatal("an in-flight section must wait for its result, marked redo")
	}
	e.meshRes <- &mesh.Mesh{Pos: near}
	frame()
	if !jobs()[near] {
		t.Fatal("the redone section was not dispatched the frame its old mesh arrived")
	}
}
