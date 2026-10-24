// Package blocktick runs a world's random and scheduled block ticks the way
// the reference server does: every tick the scheduled ticks that are due
// (WorldServer.tickUpdates), then three random ticks per chunk section
// around the players (WorldServer.func_147456_g).
package blocktick

import (
	"cmp"
	"container/heap"
	"math/bits"
	"slices"

	"github.com/memmaker/voxellib/voxel"
)

// MaxPerTick is how many scheduled ticks run in one tick at most.
const MaxPerTick = 1000

// RandomPerSection is how many random ticks a section gets per tick.
const RandomPerSection = 3

// Ticks is a world's tick state. Block identity is the caller's: BlockOf
// maps a state to its block (0 for air), so two states of one block (its
// metadata, or blocks the reference treats as associated, such as the lit
// and unlit diode) share a scheduled tick.
type Ticks struct {
	W       *voxel.World
	BlockOf func(voxel.StateID) int32
	// Random holds, per state, whether it ticks randomly (getTickRandomly).
	Random []bool
	// OnRandom and OnScheduled are the block's updateTick.
	OnRandom    func(x, y, z int, id voxel.StateID)
	OnScheduled func(x, y, z int, id voxel.StateID)

	// LCG is the world's updateLCG (the reference seeds it from a random int).
	LCG int32

	pending map[key]*Entry
	queue   entries
	seq     int64
	due     []*Entry
	next    int // the index in due of the next tick to run
	parked  map[voxel.ChunkPos][]*Entry
	picks   []pick
	flags   *voxel.TickFlags // Random as the chunks count it
	nflags  int
}

type key struct {
	p     voxel.BlockPos
	block int32
}

type pick struct{ x, y, z int }

// Entry is a scheduled tick (NextTickListEntry).
type Entry struct {
	Pos      voxel.BlockPos
	Block    int32
	Time     int64 // the world time it is due
	Priority int
	seq      int64
	i        int
}

// New makes the tick state of world w.
func New(w *voxel.World, blockOf func(voxel.StateID) int32) *Ticks {
	return &Ticks{W: w, BlockOf: blockOf, pending: map[key]*Entry{}, parked: map[voxel.ChunkPos][]*Entry{}}
}

func (t *Ticks) loaded(p voxel.BlockPos) bool {
	return t.W.HasChunk(voxel.ChunkPos{X: p.X >> 4, Z: p.Z >> 4})
}

// Schedule is scheduleBlockUpdateWithPriority: a tick of block at p in
// delay ticks from now, unless one is already pending for it there. A
// position whose chunk is not loaded gets none.
func (t *Ticks) Schedule(p voxel.BlockPos, block int32, delay, priority int, now int64) {
	if !t.loaded(p) {
		return
	}
	t.add(p, block, int64(delay)+now, priority)
}

// Load is scheduleBlockUpdateFromLoad: a tick read from a saved chunk,
// added even while its chunk is not in the world yet.
func (t *Ticks) Load(p voxel.BlockPos, block int32, delay, priority int, now int64) {
	t.add(p, block, int64(delay)+now, priority)
}

func (t *Ticks) add(p voxel.BlockPos, block int32, time int64, priority int) {
	k := key{p, block}
	if t.pending[k] != nil {
		return
	}
	e := &Entry{Pos: p, Block: block, Time: time, Priority: priority, seq: t.seq}
	t.seq++
	t.pending[k] = e
	heap.Push(&t.queue, e)
}

// Pending reports whether a tick of block at p waits in the queue.
func (t *Ticks) Pending(p voxel.BlockPos, block int32) bool { return t.pending[key{p, block}] != nil }

// Len is the number of waiting ticks.
func (t *Ticks) Len() int { return len(t.queue) }

// Entries returns the waiting ticks at world time now for saving, in
// order, each Time the delay left (the chunk NBT's TileTicks "t": Load
// takes them back).
func (t *Ticks) Entries(now int64) []Entry {
	out := make([]Entry, 0, len(t.queue))
	for _, e := range t.queue {
		c := *e
		c.Time -= now
		out = append(out, c)
	}
	for _, es := range t.parked {
		for _, e := range es {
			out = append(out, *e) // parked with the delay left
		}
	}
	slices.SortStableFunc(out, func(a, b Entry) int { return cmp.Compare(a.seq, b.seq) })
	return out
}

// RunScheduled is tickUpdates(false) at world time now: up to MaxPerTick
// due ticks leave the queue in order (time, priority, then the order they
// were scheduled), then each runs if its block is still there.
//
// The reference saves a chunk's pending ticks when the chunk unloads and
// adds them back when it loads; here a due tick whose chunk is not loaded
// is parked with that chunk until ChunkLoaded.
func (t *Ticks) RunScheduled(now int64) {
	n := min(len(t.queue), MaxPerTick)
	t.due = t.due[:0]
	for range n {
		e := t.queue[0]
		if e.Time > now {
			break
		}
		heap.Pop(&t.queue)
		delete(t.pending, key{e.Pos, e.Block})
		t.due = append(t.due, e)
	}
	for i, e := range t.due {
		t.next = i + 1
		if !t.loaded(e.Pos) {
			c := voxel.ChunkPos{X: e.Pos.X >> 4, Z: e.Pos.Z >> 4}
			e.Time -= now // the delay left, taken up again on load
			t.parked[c] = append(t.parked[c], e)
			continue
		}
		id := t.W.Block(e.Pos.X, e.Pos.Y, e.Pos.Z)
		if b := t.BlockOf(id); b != 0 && b == e.Block && t.OnScheduled != nil {
			t.OnScheduled(e.Pos.X, e.Pos.Y, e.Pos.Z, id)
		}
	}
	t.due, t.next = t.due[:0], 0
}

// ScheduledThisTick is isBlockTickScheduledThisTick: a tick of block at p
// left the queue this tick and has not run yet.
func (t *Ticks) ScheduledThisTick(p voxel.BlockPos, block int32) bool {
	for _, e := range t.due[t.next:] {
		if e.Pos == p && e.Block == block {
			return true
		}
	}
	return false
}

// ChunkLoaded takes up the ticks parked with chunk c again.
func (t *Ticks) ChunkLoaded(c voxel.ChunkPos, now int64) {
	for _, e := range t.parked[c] {
		t.add(e.Pos, e.Block, e.Time+now, e.Priority)
	}
	delete(t.parked, c)
}

// RunRandom is the random block ticks of func_147456_g for the loaded
// chunks within radius chunks of each centre (each chunk once): per
// section that holds a randomly ticking state (Random only grows), RandomPerSection picks of
// the updateLCG, each running the picked block's tick if it ticks randomly.
func (t *Ticks) RunRandom(centres []voxel.ChunkPos, radius int) {
	if t.flags == nil {
		t.flags = voxel.NewTickFlags(slices.Clone(t.Random))
	} else if len(t.Random) != t.nflags {
		t.flags = t.flags.Grow(slices.Clone(t.Random))
	}
	t.nflags = len(t.Random)
	seen := map[voxel.ChunkPos]bool{}
	for _, c := range centres {
		for dx := -radius; dx <= radius; dx++ {
			for dz := -radius; dz <= radius; dz++ {
				p := voxel.ChunkPos{X: c.X + dx, Z: c.Z + dz}
				if len(centres) > 1 {
					if seen[p] {
						continue
					}
					seen[p] = true
				}
				t.randomChunk(p)
			}
		}
	}
}

func (t *Ticks) mayTick(s voxel.StateID) bool { return int(s) < len(t.Random) && t.Random[s] }

func (t *Ticks) randomChunk(p voxel.ChunkPos) {
	w := t.W
	w.RLock()
	c := w.ChunkUnlocked(p)
	if c == nil {
		w.RUnlock()
		return
	}
	// The picks are made under the read lock and run after it: a tick
	// changes blocks, and the picked positions do not depend on them.
	t.picks = t.picks[:0]
	x0, z0 := p.X<<4, p.Z<<4
	for m := c.Ticking(t.flags); m != 0; m &= m - 1 {
		i := bits.TrailingZeros64(m)
		s := c.Sections[i]
		y0 := c.MinY() + i<<4
		for range RandomPerSection {
			t.LCG = t.LCG*3 + 1013904223
			r := t.LCG >> 2
			x, z, y := int(r&15), int(r>>8&15), int(r>>16&15)
			if s.TickingAt(voxel.SectionIndex(x, y, z)) {
				t.picks = append(t.picks, pick{x0 + x, y0 + y, z0 + z})
			}
		}
	}
	w.RUnlock()
	for _, k := range t.picks {
		// an earlier tick of this chunk may have changed it
		if id := w.Block(k.x, k.y, k.z); t.mayTick(id) && t.OnRandom != nil {
			t.OnRandom(k.x, k.y, k.z, id)
		}
	}
}

// entries is the queue ordered as NextTickListEntry.compareTo.
type entries []*Entry

func (q entries) Len() int { return len(q) }
func (q entries) Less(i, j int) bool {
	a, b := q[i], q[j]
	if a.Time != b.Time {
		return a.Time < b.Time
	}
	if a.Priority != b.Priority {
		return a.Priority < b.Priority
	}
	return a.seq < b.seq
}
func (q entries) Swap(i, j int) { q[i], q[j] = q[j], q[i]; q[i].i = i; q[j].i = j }
func (q *entries) Push(x any)   { e := x.(*Entry); e.i = len(*q); *q = append(*q, e) }
func (q *entries) Pop() any {
	old := *q
	e := old[len(old)-1]
	*q = old[:len(old)-1]
	return e
}
