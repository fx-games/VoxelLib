// Command mpbot is a headless LAN multiplayer player for testing: it joins
// a game, walks (along a voxel/nav path) up to the nearest other player until it stands -dist
// blocks in front of them (or -angle round), faces them, and optionally shoots (-shoot) with
// a ballistic aim at their head.
//
//	go run ./cmd/mpbot -connect 127.0.0.1 -skin skins/ArrowJay.png
package main

import (
	"flag"
	"fmt"
	"log"
	"math"
	"os"
	"strings"
	"time"

	"github.com/memmaker/voxellib/actors"
	"github.com/memmaker/voxellib/character"
	"github.com/memmaker/voxellib/mp"
	"github.com/memmaker/voxellib/netcode"
	"github.com/memmaker/voxellib/projectile"
	"github.com/memmaker/voxellib/voxel"
	"github.com/memmaker/voxellib/voxel/nav"
)

// pather follows a nav path to a goal, planned again each second or when
// the goal moves to another cell.
type pather struct {
	g    *nav.Graph
	path []nav.Pos
	goal nav.Pos
	at   time.Time
}

// next is the point to walk to on the way to x, y, z, and whether it is a
// step up.
func (p *pather) next(w *mp.World, ch *character.Character, x, y, z float64) (wx, wz float64, up bool) {
	goal := nav.Pos{X: int(math.Floor(x)), Y: int(math.Floor(y)), Z: int(math.Floor(z))}
	if goal != p.goal || time.Since(p.at) > time.Second {
		p.goal, p.at = goal, time.Now()
		w.LoadAround(ch.X, ch.Z, 48)
		from := nav.Pos{X: int(math.Floor(ch.X)), Y: int(math.Floor(ch.Y + 0.01)), Z: int(math.Floor(ch.Z))}
		if g, ok := p.g.Ground(goal.X, goal.Y, goal.Z); ok {
			goal = g
		}
		p.path, _ = p.g.Path(from, goal, 4000)
	}
	// Drop the cells reached; head for the first one not yet.
	for len(p.path) > 0 {
		c := p.path[0]
		if math.Hypot(float64(c.X)+0.5-ch.X, float64(c.Z)+0.5-ch.Z) > 0.35 {
			break
		}
		p.path = p.path[1:]
	}
	if len(p.path) == 0 {
		return x, z, false
	}
	c := p.path[0]
	return float64(c.X) + 0.5, float64(c.Z) + 0.5, float64(c.Y) > ch.Y+0.6
}

func main() {
	addr := flag.String("connect", "127.0.0.1", "server address (host[:port]); '': the first LAN game found")
	name := flag.String("name", "Bot", "player name")
	skinPath := flag.String("skin", "", "skin PNG to send")
	dist := flag.Float64("dist", 4, "distance to keep in front of the target")
	angle := flag.Float64("angle", 0, "stand this many degrees round from the target's look (90: to their right)")
	target := flag.String("target", "", "only go for the player with this name ('' = the nearest)")
	shoot := flag.Bool("shoot", false, "shoot at the target's head once in place")
	limit := flag.Duration("time", time.Minute, "leave after this long")
	flag.Parse()
	if *addr == "" {
		found := mp.Discover(time.Second)
		if len(found) == 0 {
			log.Fatal("no LAN game found")
		}
		for _, f := range found {
			log.Printf("found %q at %s (%d/%d, %v)", f.Name, f.Addr, f.Players, f.Max, f.Ping)
		}
		*addr = found[0].Addr
	}
	if !strings.Contains(*addr, ":") {
		*addr = fmt.Sprintf("%s:%d", *addr, mp.DefaultPort)
	}
	c, err := mp.Connect(*addr, *name, netcode.Config{})
	if err != nil {
		log.Fatal(err)
	}
	defer c.Close()
	if *skinPath != "" {
		b, err := os.ReadFile(*skinPath)
		if err != nil {
			log.Fatal(err)
		}
		c.SendSkin(b)
	}
	reg := voxel.NewRegistry()
	gen, err := mp.NewGenerator(reg, c.Welcome.Generator, c.Welcome.Seed)
	if err != nil {
		log.Fatal(err)
	}
	minY, height := mp.Heights(c.Welcome.Generator)
	w := mp.NewWorld(reg, c.Edits.Wrap(gen), minY, height)
	c.OnBlock = func(e mp.BlockEdit) { w.SetBlock(e.X, e.Y, e.Z, reg.Parse(e.State)) }
	c.OnText = func(s string) { log.Printf("text: %s", s) }
	c.OnHit = func(h mp.Hit) { log.Printf("hit: %+v", h) }
	ch := character.New(0, 0, 0)
	c.Welcome.Own.Apply(ch)
	log.Printf("joined as slot %d", c.Slot())

	end := time.Now().Add(*limit)
	tick := time.NewTicker(time.Second / mp.TickRate)
	var yaw, pitch float64
	var lastLog time.Time
	bestD, bestAt := math.Inf(1), time.Now()
	pf := &pather{g: (&actors.Terrain{W: w.W}).Graph(nav.SizeOf(0.6, 1.8))}
	pf.g.Closest, pf.g.Door = true, nil // a bot opens no doors
	for time.Now().Before(end) {
		<-tick.C
		c.Poll()
		if r := c.Gone(); r != "" {
			log.Fatalf("gone: %s", r)
		}
		c.Reconcile(ch, w)
		in := character.Input{Yaw: yaw, Pitch: pitch}
		attack := false
		r := c.Round()
		z := r.Zone
		if r.Mode == mp.BattleRoyale && r.Phase == mp.PhasePlaying && z.Touches(ch.X, ch.Z, math.Min(6, z.Side()/4)) {
			// Away from the wall: to the middle.
			dx, dz := (z.MinX+z.MaxX)/2-ch.X, (z.MinZ+z.MaxZ)/2-ch.Z
			in.Yaw, in.Forward, in.Sprint = math.Atan2(dx, -dz), 1, true
			in.Jump = ch.HorizontalCollision
		} else if t, ok := nearest(c, ch, *target); ok {
			// The spot -dist in front of the target, -angle round from their look.
			a := t.Yaw + *angle*math.Pi/180
			gx, gz := t.X+math.Sin(a)**dist, t.Z-math.Cos(a)**dist
			dx, dz := gx-ch.X, gz-ch.Z
			d := math.Hypot(dx, dz)
			if d < bestD-0.5 {
				bestD, bestAt = d, time.Now()
			}
			stuck := time.Since(bestAt) > 3*time.Second // shoot from here
			if d > 0.4 && !stuck {
				// Along a path to the spot, else straight at it.
				wx, wz, up := pf.next(w, ch, gx, t.Y, gz)
				in.Yaw = math.Atan2(wx-ch.X, -(wz - ch.Z))
				in.Forward = math.Min(1, d)
				in.Sprint = d > 6
				in.Jump = up || ch.HorizontalCollision // a step up in the way
			} else {
				ex, ey, ez := ch.Eye()
				in.Yaw, in.Pitch = aim([3]float64{ex, ey - 0.1, ez}, [3]float64{t.X, t.Y + 1.75, t.Z})
				attack = *shoot
			}
			if time.Since(lastLog) > 2*time.Second {
				lastLog = time.Now()
				log.Printf("at %.1f %.1f %.1f, target slot %d at %.1f %.1f %.1f (%.1f to go)", ch.X, ch.Y, ch.Z, t.Slot, t.X, t.Y, t.Z, d)
			}
		}
		yaw, pitch = in.Yaw, in.Pitch
		cmd := c.Command(in, attack)
		if !c.Own().Dead {
			ch.Tick(w, in)
		}
		c.Predicted(cmd, ch, w)
	}
}

func nearest(c *mp.Client, ch *character.Character, name string) (mp.PlayerView, bool) {
	var best mp.PlayerView
	bd := math.Inf(1)
	for _, v := range c.Players() {
		if name != "" && c.Info[v.Slot].Name != name {
			continue
		}
		if team := c.Info[c.Slot()].Team; team != 0 && c.Info[v.Slot].Team == team {
			continue // a teammate
		}
		if d := math.Hypot(v.X-ch.X, v.Z-ch.Z); !v.Dead && !v.Protected && d < bd {
			best, bd = v, d
		}
	}
	return best, !math.IsInf(bd, 1)
}

// aim returns the look that sends an arrow from eye to target with the
// real physics (as mp's test).
func aim(eye, target [3]float64) (yaw, pitch float64) {
	dx, dy, dz := target[0]-eye[0], target[1]-eye[1], target[2]-eye[2]
	yaw = math.Atan2(dx, -dz)
	dist := math.Hypot(dx, dz)
	heightAt := func(p float64) float64 {
		var w projectile.World
		a := w.Shoot(0, 0, 0, 0, p, projectile.Speed)
		for range 400 {
			px, py := -a.Z, a.Y
			w.Tick(noBlocks{}, nil)
			if -a.Z >= dist {
				f := (dist - px) / (-a.Z - px)
				return py + (a.Y-py)*f
			}
		}
		return math.Inf(-1)
	}
	lo, hi := -0.8, 0.8
	for range 50 {
		pitch = (lo + hi) / 2
		if heightAt(pitch) < dy {
			lo = pitch
		} else {
			hi = pitch
		}
	}
	return yaw, pitch
}

type noBlocks struct{}

func (noBlocks) Solid(x, y, z int) bool { return false }
