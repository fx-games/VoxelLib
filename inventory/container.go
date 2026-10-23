package inventory

// Slot is one clickable slot of a container, showing slot Index of Inv.
type Slot struct {
	Inv   Inventory
	Index int
	// Accept, if set, decides isItemValid (armour slots, output slots).
	Accept func(*Stack) bool
	// Max, if > 0, caps the stack size below Inv.Limit() (armour: 1).
	Max int
	// Output marks a result slot (crafting, furnace): it accepts nothing and
	// counts what is taken so OnTake learns the amount produced.
	Output bool
	// OnTake runs after the player took s from the slot (onPickupFromSlot).
	// For output slots, n is the number of items produced since the last call.
	OnTake func(c *Container, s *Stack, n int)

	taken int
}

// Stack returns the slot's stack.
func (sl *Slot) Stack() *Stack { return sl.Inv.Get(sl.Index) }

// Has reports a non-empty slot.
func (sl *Slot) Has() bool { return sl.Stack() != nil }

// Put sets the slot's stack.
func (sl *Slot) Put(s *Stack) { sl.Inv.Set(sl.Index, s); sl.Changed() }

// Changed marks the inventory dirty.
func (sl *Slot) Changed() { sl.Inv.Changed() }

// Valid reports isItemValid.
func (sl *Slot) Valid(s *Stack) bool {
	if sl.Output {
		return false
	}
	return sl.Accept == nil || sl.Accept(s)
}

// Limit returns getSlotStackLimit.
func (sl *Slot) Limit() int {
	if sl.Max > 0 {
		return sl.Max
	}
	return sl.Inv.Limit()
}

// Take is decrStackSize.
func (sl *Slot) Take(n int) *Stack {
	if sl.Output && sl.Has() {
		sl.taken += min(n, sl.Stack().Count)
	}
	return Take(sl.Inv, sl.Index, n)
}

// crafted is onSlotChange: after a shift-click merge, the moved amount counts
// as produced.
func (sl *Slot) crafted(after, before *Stack) {
	if after != nil && before != nil && after.Item == before.Item {
		if d := before.Count - after.Count; d > 0 && sl.Output {
			sl.taken += d
		}
	}
}

func (sl *Slot) pickedUp(c *Container, s *Stack) {
	n := sl.taken
	sl.taken = 0
	if sl.OnTake != nil {
		sl.OnTake(c, s, n)
	}
	sl.Changed()
}

// Click modes of Container.slotClick.
const (
	Pickup    = 0 // left/right click
	QuickMove = 1 // shift-click
	Swap      = 2 // number key: button is the hotbar slot 0–8
	Clone     = 3 // middle click in creative
	Throw     = 4 // Q over a slot: button 0 one item, 1 the stack
	Drag      = 5 // drag-split: button from DragButton
	PickupAll = 6 // double click
)

// Outside is the slot id for a click outside the window (drops the cursor).
const Outside = -999

// Drag kinds and stages for DragButton.
const (
	DragEven = 0 // left drag: split evenly
	DragOne  = 1 // right drag: one per slot

	DragStart = 0
	DragAdd   = 1
	DragEnd   = 2
)

// DragButton encodes a drag click's button (Container.func_94534_d).
func DragButton(kind, stage int) int { return stage&3 | (kind&3)<<2 }

// Container is a window of slots operated by one player (Container).
type Container struct {
	Slots  []*Slot
	Player *Player
	// Transfer is transferStackInSlot (shift-click); nil does nothing.
	Transfer func(c *Container, i int) *Stack
	// CanCollect is func_94530_a: may double-click collect take from sl?
	CanCollect func(s *Stack, sl *Slot) bool
	// Drop throws a stack into the world (dropPlayerItemWithRandomChoice).
	Drop func(*Stack)
	// OnClose runs after the cursor was dropped on Close.
	OnClose func(c *Container)
	// Deny, if set, is a slotClick override: a click it denies does nothing
	// and returns nil (LOTRContainerPouch locks the open pouch's slot).
	Deny func(slot, button, mode int) bool

	dragKind, dragStage int
	dragSlots           []*Slot
}

// Add appends a slot and returns it.
func (c *Container) Add(sl *Slot) *Slot { c.Slots = append(c.Slots, sl); return sl }

func (c *Container) drop(s *Stack) {
	if c.Drop != nil && s != nil && s.Count > 0 {
		c.Drop(s)
	}
}

func (c *Container) resetDrag() { c.dragStage = 0; c.dragSlots = c.dragSlots[:0] }

// fits is func_94527_a.
func fits(sl *Slot, s *Stack, sizeMatters bool) bool {
	ok := sl == nil || !sl.Has()
	if sl != nil && sl.Has() && s != nil && s.Same(sl.Stack()) && TagsEqual(sl.Stack(), s) {
		extra := s.Count
		if sizeMatters {
			extra = 0
		}
		ok = ok || sl.Stack().Count+extra <= s.MaxStack()
	}
	return ok
}

func (c *Container) inDrag(sl *Slot) bool {
	for _, d := range c.dragSlots {
		if d == sl {
			return true
		}
	}
	return false
}

// Click is Container.slotClick: it returns a copy of the clicked slot's stack
// before the click (nil for none), as 1.7.10 does.
func (c *Container) Click(slot, button, mode int) *Stack {
	if c.Deny != nil && c.Deny(slot, button, mode) {
		return nil
	}
	var ret *Stack
	p := c.Player
	if mode == Drag {
		prev := c.dragStage
		c.dragStage = button & 3
		switch {
		case (prev != 1 || c.dragStage != 2) && prev != c.dragStage:
			c.resetDrag()
		case p.Cursor == nil:
			c.resetDrag()
		case c.dragStage == 0:
			c.dragKind = button >> 2 & 3
			if c.dragKind == DragEven || c.dragKind == DragOne {
				c.dragStage = 1
				c.dragSlots = c.dragSlots[:0]
			} else {
				c.resetDrag()
			}
		case c.dragStage == 1:
			if slot < 0 || slot >= len(c.Slots) {
				break
			}
			sl := c.Slots[slot]
			if fits(sl, p.Cursor, true) && sl.Valid(p.Cursor) && p.Cursor.Count > len(c.dragSlots) && !c.inDrag(sl) {
				c.dragSlots = append(c.dragSlots, sl)
			}
		case c.dragStage == 2:
			if len(c.dragSlots) > 0 {
				src := p.Cursor.Copy()
				left := p.Cursor.Count
				for _, sl := range c.dragSlots {
					if fits(sl, p.Cursor, true) && sl.Valid(p.Cursor) && p.Cursor.Count >= len(c.dragSlots) {
						s := src.Copy()
						had := 0
						if sl.Has() {
							had = sl.Stack().Count
						}
						if c.dragKind == DragEven {
							s.Count = int(float32(s.Count) / float32(len(c.dragSlots)))
						} else {
							s.Count = 1
						}
						s.Count += had
						s.Count = min(s.Count, s.MaxStack(), sl.Limit())
						left -= s.Count - had
						sl.Put(s)
					}
				}
				src.Count = left
				if src.Count <= 0 {
					src = nil
				}
				p.Cursor = src
			}
			c.resetDrag()
		default:
			c.resetDrag()
		}
		return nil
	}
	if c.dragStage != 0 {
		c.resetDrag()
		return nil
	}

	switch {
	case (mode == Pickup || mode == QuickMove) && (button == 0 || button == 1):
		if slot == Outside {
			if p.Cursor != nil {
				if button == 0 {
					c.drop(p.Cursor)
					p.Cursor = nil
				} else {
					c.drop(p.Cursor.Split(1))
					if p.Cursor.Count == 0 {
						p.Cursor = nil
					}
				}
			}
			return nil
		}
		if slot < 0 {
			return nil
		}
		sl := c.Slots[slot]
		if mode == QuickMove {
			if c.Transfer == nil {
				return nil
			}
			if moved := c.Transfer(c, slot); moved != nil {
				ret = moved.Copy()
				if s := sl.Stack(); s != nil && s.Item == moved.Item {
					c.Click(slot, button, QuickMove) // retrySlotClick
				}
			}
			return ret
		}
		in, cur := sl.Stack(), p.Cursor
		if in != nil {
			ret = in.Copy()
		}
		switch {
		case in == nil:
			if cur != nil && sl.Valid(cur) {
				n := cur.Count
				if button == 1 {
					n = 1
				}
				n = min(n, sl.Limit())
				if cur.Count >= n {
					sl.Put(cur.Split(n))
				}
				if cur.Count == 0 {
					p.Cursor = nil
				}
			}
		case cur == nil:
			n := in.Count
			if button == 1 {
				n = (in.Count + 1) / 2
			}
			p.Cursor = sl.Take(n)
			if in.Count == 0 {
				sl.Put(nil)
			}
			sl.pickedUp(c, p.Cursor)
		case sl.Valid(cur):
			if in.Item == cur.Item && in.Damage == cur.Damage && TagsEqual(in, cur) {
				n := cur.Count
				if button == 1 {
					n = 1
				}
				n = min(n, sl.Limit()-in.Count, cur.MaxStack()-in.Count)
				cur.Split(n)
				if cur.Count == 0 {
					p.Cursor = nil
				}
				in.Count += n
			} else if cur.Count <= sl.Limit() {
				sl.Put(cur)
				p.Cursor = in
			}
		case in.Item == cur.Item && cur.MaxStack() > 1 && (!in.Item.HasSubtypes || in.Damage == cur.Damage) && TagsEqual(in, cur):
			// taking from an output slot into a matching cursor stack
			if n := in.Count; n > 0 && n+cur.Count <= cur.MaxStack() {
				cur.Count += n
				in = sl.Take(n)
				if in.Count == 0 {
					sl.Put(nil)
				}
				sl.pickedUp(c, p.Cursor)
			}
		}
		sl.Changed()

	case mode == Swap && button >= 0 && button < 9:
		sl := c.Slots[slot]
		hot := p.Main[button]
		ok := hot == nil || sl.Inv == Inventory(p) && sl.Valid(hot)
		empty := -1
		if !ok {
			empty = p.FirstEmpty()
			ok = empty > -1
		}
		if sl.Has() && ok {
			in := sl.Stack()
			p.Main[button] = in.Copy()
			if (sl.Inv != Inventory(p) || !sl.Valid(hot)) && hot != nil {
				if empty > -1 {
					p.Add(hot)
					sl.Take(in.Count)
					sl.Put(nil)
					sl.pickedUp(c, in)
				}
			} else {
				sl.Take(in.Count)
				sl.Put(hot)
				sl.pickedUp(c, in)
			}
		} else if !sl.Has() && hot != nil && sl.Valid(hot) {
			p.Main[button] = nil
			sl.Put(hot)
		}

	case mode == Clone && p.Creative && p.Cursor == nil && slot >= 0:
		if s := c.Slots[slot].Stack(); s != nil {
			s = s.Copy()
			s.Count = s.MaxStack()
			p.Cursor = s
		}

	case mode == Throw && p.Cursor == nil && slot >= 0:
		sl := c.Slots[slot]
		if s := sl.Stack(); s != nil {
			n := s.Count
			if button == 0 {
				n = 1
			}
			t := sl.Take(n)
			sl.pickedUp(c, t)
			c.drop(t)
		}

	case mode == PickupAll && slot >= 0:
		sl := c.Slots[slot]
		cur := p.Cursor
		if cur != nil && (sl == nil || !sl.Has()) {
			start, step := 0, 1
			if button != 0 {
				start, step = len(c.Slots)-1, -1
			}
			for pass := 0; pass < 2; pass++ {
				for j := start; j >= 0 && j < len(c.Slots) && cur.Count < cur.MaxStack(); j += step {
					o := c.Slots[j]
					if o.Has() && fits(o, cur, true) && (c.CanCollect == nil || c.CanCollect(cur, o)) &&
						(pass != 0 || o.Stack().Count != o.Stack().MaxStack()) {
						k := min(cur.MaxStack()-cur.Count, o.Stack().Count)
						t := o.Take(k)
						cur.Count += k
						if t.Count <= 0 {
							o.Put(nil)
						}
						o.pickedUp(c, t)
					}
				}
			}
		}
	}
	return ret
}

// Merge is mergeItemStack: move s into slots [from,to), filling matching
// stacks first, then the first empty slot; reverse walks backwards. It
// reduces s.Count and reports whether anything moved.
func (c *Container) Merge(s *Stack, from, to int, reverse bool) bool {
	moved := false
	k, step := from, 1
	if reverse {
		k, step = to-1, -1
	}
	in := func() bool { return !reverse && k < to || reverse && k >= from }
	if s.Stackable() {
		for s.Count > 0 && in() {
			sl := c.Slots[k]
			if o := sl.Stack(); o != nil && o.Item == s.Item && (!s.Item.HasSubtypes || s.Damage == o.Damage) && TagsEqual(s, o) {
				if t := o.Count + s.Count; t <= s.MaxStack() {
					s.Count = 0
					o.Count = t
					sl.Changed()
					moved = true
				} else if o.Count < s.MaxStack() {
					s.Count -= s.MaxStack() - o.Count
					o.Count = s.MaxStack()
					sl.Changed()
					moved = true
				}
			}
			k += step
		}
	}
	if s.Count > 0 {
		k = from
		if reverse {
			k = to - 1
		}
		for in() {
			sl := c.Slots[k]
			if !sl.Has() {
				sl.Put(s.Copy())
				s.Count = 0
				moved = true
				break
			}
			k += step
		}
	}
	return moved
}

// Close is onContainerClosed: the cursor stack is dropped, then OnClose runs.
func (c *Container) Close() {
	if c.Player.Cursor != nil {
		c.drop(c.Player.Cursor)
		c.Player.Cursor = nil
	}
	if c.OnClose != nil {
		c.OnClose(c)
	}
}

// transferWith is the common body of the vanilla transferStackInSlot
// implementations: route decides the merge, output slots get onSlotChange.
func transferWith(c *Container, i int, route func(s *Stack) bool, checkSame bool) *Stack {
	sl := c.Slots[i]
	s := sl.Stack()
	if s == nil {
		return nil
	}
	before := s.Copy()
	if !route(s) {
		return nil
	}
	if sl.Output {
		sl.crafted(s, before)
	}
	if s.Count == 0 {
		sl.Put(nil)
	} else {
		sl.Changed()
	}
	if checkSame {
		if s.Count == before.Count {
			return nil
		}
		sl.pickedUp(c, s)
	}
	return before
}
