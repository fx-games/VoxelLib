package ui

import "time"

// Notification is one window of the reference's corner notification
// display: a sheet region that slides down from the top right corner,
// stays, and slides back up over Duration.
type Notification struct {
	Sheet      string // background texture, addressed as 256x256
	U, V, W, H int    // its region
	Duration   time.Duration
	// Draw draws the text and icon with the window's top-left at x, y.
	Draw  func(ctx *Context, x, y int)
	start time.Time
}

// Notifications is the queue; all windows show at once, stacked H+8 apart
// by their place in the queue.
type Notifications struct {
	list []*Notification
}

// Add queues n, starting now.
func (q *Notifications) Add(n *Notification, now time.Time) {
	n.start = now
	q.list = append(q.list, n)
}

// Len is the number of queued windows.
func (q *Notifications) Len() int { return len(q.list) }

// Draw draws the windows at time now and drops the finished ones.
func (q *Notifications) Draw(ctx *Context, now time.Time) {
	keep := q.list[:0]
	for i, n := range q.list {
		d := float64(now.Sub(n.start).Milliseconds()) / float64(n.Duration.Milliseconds())
		if d < 0 || d > 1 {
			continue
		}
		keep = append(keep, n)
		x := ctx.W - n.W
		y := -int(slide(d)*36) + i*(n.H+8)
		DrawTexRect(ctx.Canvas, ctx.Tex(n.Sheet), x, y, n.U, n.V, n.W, n.H, 256, 256, 0xFFFFFFFF)
		if n.Draw != nil {
			n.Draw(ctx, x, y)
		}
	}
	clear(q.list[len(keep):])
	q.list = keep
}

// slide is the window's hidden part (0 shown, 1 above the screen) at
// progress d of its duration: in during the first eighth, out during the
// last.
func slide(d float64) float64 {
	d1 := d * 2
	if d1 > 1 {
		d1 = 2 - d1
	}
	d1 = 1 - d1*4
	if d1 < 0 {
		d1 = 0
	}
	d1 *= d1
	return d1 * d1
}
