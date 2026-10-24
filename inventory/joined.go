package inventory

// Joined is InventoryLargeChest: two inventories shown as one, Upper's
// slots first, then Lower's.
type Joined struct{ Upper, Lower Inventory }

func (j *Joined) Size() int { return j.Upper.Size() + j.Lower.Size() }
func (j *Joined) Get(i int) *Stack {
	if n := j.Upper.Size(); i >= n {
		return j.Lower.Get(i - n)
	}
	return j.Upper.Get(i)
}
func (j *Joined) Set(i int, s *Stack) {
	if n := j.Upper.Size(); i >= n {
		j.Lower.Set(i-n, s)
		return
	}
	j.Upper.Set(i, s)
}
func (j *Joined) Limit() int { return j.Upper.Limit() }
func (j *Joined) Changed()   { j.Upper.Changed(); j.Lower.Changed() }
