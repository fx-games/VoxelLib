// Command mpbot is a headless LAN multiplayer player for testing: it joins
// a game, walks up to the nearest other player until it stands -dist
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

	"github.com/memmaker/voxellib/character"
	"github.com/memmaker/voxellib/mp"
	"github.com/memmaker/voxellib/netcode"
	"github.com/memmaker/voxellib/projectile"
	"github.com/memmaker/voxellib/voxel"
)

func main() {
	addr := flag.String("connect", "127.0.0.1", "server address (host[:port])")
	name := flag.String("name", "Bot", "player name")
	skinPath := flag.String("skin", "", "skin PNG to send")
	dist := flag.Float64("dist", 4, "distance to keep in front of the target")
	angle := flag.Float64("angle", 0, "stand this many degrees round from the target's look (90: to their right)")
	shoot := flag.Bool("shoot", false, "shoot at the target's head once in place")
	limit := flag.Duration("time", time.Minute, "leave after this long")
	flag.Parse()
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
	w := mp.NewWorld(reg, c.Edits.Wrap(gen))
	c.OnBlock = func(e mp.BlockEdit) { w.SetBlock(e.X, e.Y, e.Z, reg.Parse(e.State)) }
	c.OnText = func(s string) { log.Printf("text: %s", s) }
	c.OnHit = func(h mp.Hit) { log.Printf("hit: %+v", h) }
	ch := character.New(c.Welcome.SpawnX, c.Welcome.SpawnY, c.Welcome.SpawnZ)
	log.Printf("joined as slot %d", c.Slot())

	end := time.Now().Add(*limit)
	tick := time.NewTicker(time.Second / mp.TickRate)
	var yaw, pitch float64
	var lastLog time.Time
	bestD, bestAt := math.Inf(1), time.Now()
	for time.Now().Before(end) {
		<-tick.C
		c.Poll()
		if r := c.Gone(); r != "" {
			log.Fatalf("gone: %s", r)
		}
		c.Reconcile(ch, w)
		in := character.Input{Yaw: yaw, Pitch: pitch}
		attack := false
		if t, ok := nearest(c, ch); ok {
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
				in.Yaw = math.Atan2(dx, -dz)
				in.Forward = math.Min(1, d)
				in.Sprint = d > 6
				in.Jump = ch.HorizontalCollision // a step up in the way
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

func nearest(c *mp.Client, ch *character.Character) (mp.PlayerView, bool) {
	var best mp.PlayerView
	bd := math.Inf(1)
	for _, v := range c.Players() {
		if d := math.Hypot(v.X-ch.X, v.Z-ch.Z); !v.Dead && d < bd {
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
