package mp

import (
	"testing"
	"time"

	"github.com/memmaker/voxellib/netcode"
	"github.com/memmaker/voxellib/projectile"
	"github.com/memmaker/voxellib/voxel"
	"github.com/memmaker/voxellib/voxel/physics"
	"github.com/memmaker/voxellib/weapon"
)

// TestBlockDamage: shots crack a block (the client sees the stages) and
// break it once their progress reaches 1; a blast with destroy_block
// breaks the ground at its centre, cracks some around it, leaves bedrock.
func TestBlockDamage(t *testing.T) {
	s := startServer(t, nil)
	a := newBot(t, s.Addr().String(), "A", netcode.Config{})
	var cracks []BlockDamage
	a.c.OnBlockDamage = func(d BlockDamage) { cracks = append(cracks, d) }
	run(t, 3*time.Second, nil, func(int) bool { a.c.Poll(); return a.c.Ready() })

	const x, z = 40, 40
	var top voxel.BlockPos
	var st voxel.StateID
	var hard float32
	s.call(func() {
		s.world.Load(x, z)
		y := 255
		for s.world.Block(x, y, z) == voxel.Air {
			y--
		}
		top = voxel.BlockPos{X: x, Y: y, Z: z}
		st = s.world.Block(x, y, z)
		hard = s.cfg.Reg.Info(st).Strength.Hardness
	})
	if hard <= 0 {
		t.Fatalf("top block %s has hardness %v", s.cfg.Reg.StateString(st), hard)
	}
	gun := testGun("test:rifle")
	gun.Bullet.Damage = 5
	b := &projectile.Bullet{Gun: &gun, DamageScale: 1}
	shots := 0
	for broken := false; !broken; shots++ {
		s.call(func() {
			s.shotBlock(b, physics.Hit{Block: top, State: st})
			broken = s.world.Block(top.X, top.Y, top.Z) == voxel.Air
		})
		if shots > 100 {
			t.Fatal("never broke")
		}
	}
	if want := int(hard*shotHardnessScale/5 + 0.999); shots != want {
		t.Fatalf("broke after %d shots, want %d", shots, want)
	}
	run(t, 3*time.Second, nil, func(int) bool {
		a.c.Poll()
		return len(cracks) > 0 && cracks[len(cracks)-1].Stage == NoCrack
	})
	for i, c := range cracks[:len(cracks)-1] {
		if c.Stage < 0 || c.Stage >= crackStages || i > 0 && c.Stage <= cracks[i-1].Stage || c.X != top.X || c.Y != top.Y {
			t.Fatalf("cracks %+v", cracks)
		}
	}

	var hole, bedrock bool
	n := 0
	s.call(func() {
		bed := s.cfg.Reg.Default("bedrock")
		s.world.SetBlock(x, top.Y-2, z, bed)
		at := [3]float64{x + 0.5, float64(top.Y) + 0.5, z + 0.5}
		s.blastBlocks(&weapon.Explosion{Radius: 3, DestroyBlock: true}, at)
		hole = s.world.Block(x, top.Y-1, z) == voxel.Air
		bedrock = s.world.Block(x, top.Y-2, z) == bed
		n = len(s.damaged)
	})
	if !hole || !bedrock || n == 0 {
		t.Fatalf("blast: hole %v, bedrock %v, cracked %d", hole, bedrock, n)
	}
}
