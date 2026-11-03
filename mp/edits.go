package mp

import (
	"sync"

	"github.com/memmaker/voxellib/voxel"
)

// EditLog is every block changed since the world was generated: the latest
// state per position, in order of first change. Server and clients keep
// one; generating a column through Wrap replays the edits in it, so a
// column that loads (or reloads) after an edit still shows it. Like
// Minecraft's world sync, only changes travel: everyone generates the
// rest from the seed.
type EditLog struct {
	mu    sync.Mutex
	order []voxel.BlockPos
	state map[voxel.BlockPos]string
	index map[voxel.BlockPos]int
	cols  map[voxel.ChunkPos][]voxel.BlockPos
}

// NewEditLog makes an empty log.
func NewEditLog() *EditLog {
	return &EditLog{state: map[voxel.BlockPos]string{}, index: map[voxel.BlockPos]int{}, cols: map[voxel.ChunkPos][]voxel.BlockPos{}}
}

// Set records an edit and returns its position's index in the log.
func (l *EditLog) Set(e BlockEdit) int {
	p := voxel.BlockPos{X: e.X, Y: e.Y, Z: e.Z}
	l.mu.Lock()
	defer l.mu.Unlock()
	i, ok := l.index[p]
	if !ok {
		i = len(l.order)
		l.index[p] = i
		l.order = append(l.order, p)
		c := voxel.ChunkPos{X: p.X >> 4, Z: p.Z >> 4}
		l.cols[c] = append(l.cols[c], p)
	}
	l.state[p] = e.State
	return i
}

// Len is the number of edited positions.
func (l *EditLog) Len() int {
	l.mu.Lock()
	defer l.mu.Unlock()
	return len(l.order)
}

// Range returns the edits [from, to) in order of first change.
func (l *EditLog) Range(from, to int) []BlockEdit {
	l.mu.Lock()
	defer l.mu.Unlock()
	to = min(to, len(l.order))
	out := make([]BlockEdit, 0, max(0, to-from))
	for _, p := range l.order[min(from, to):to] {
		out = append(out, BlockEdit{X: p.X, Y: p.Y, Z: p.Z, State: l.state[p]})
	}
	return out
}

// Apply writes the log's edits in column c into it (before the column is
// added to the world).
func (l *EditLog) Apply(reg *voxel.Registry, c *voxel.Chunk) {
	l.mu.Lock()
	ps := l.cols[c.Pos]
	edits := make([]BlockEdit, 0, len(ps))
	for _, p := range ps {
		edits = append(edits, BlockEdit{X: p.X, Y: p.Y, Z: p.Z, State: l.state[p]})
	}
	l.mu.Unlock()
	if len(edits) == 0 {
		return
	}
	for _, e := range edits {
		c.SetRaw(e.X&15, e.Y, e.Z&15, reg.Parse(e.State))
	}
	c.RecomputeHeights(reg.Table())
	c.Modified = true
}

// Wrap returns a generator that generates with g, then replays the log.
func (l *EditLog) Wrap(g Generator) Generator { return &replayGen{g, l} }

type replayGen struct {
	g Generator
	l *EditLog
}

func (r *replayGen) Generate(w *voxel.World, c *voxel.Chunk) {
	if r.g != nil {
		r.g.Generate(w, c)
	}
	r.l.Apply(w.Reg, c)
}

// FindLand passes through to the wrapped generator's spawn finder.
func (r *replayGen) FindLand(x, z int) (int, int) {
	if sp, ok := r.g.(interface{ FindLand(x, z int) (int, int) }); ok {
		return sp.FindLand(x, z)
	}
	return x, z
}
