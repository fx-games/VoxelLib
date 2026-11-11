package mp

import (
	"testing"

	"github.com/go-gl/mathgl/mgl32"
	"github.com/memmaker/voxellib/weapon"
)

// TestGunHold poses the arms as TACZ's default third-person hold with a gun
// in hand, and the hold survives Lerp (the server's lag compensation).
func TestGunHold(t *testing.T) {
	b := NewBody()
	bow := b.Hand(b.Skin(Anim{}, nil))
	gun := b.Hand(b.Skin(Anim{Gun: true}.Lerp(Anim{Gun: true}, 0.5), nil))
	if bow.Sub(gun).Len() < 0.05 {
		t.Fatalf("gun hold hand %v = crossbow hold %v", gun, bow)
	}
	l := (Anim{}).Lerp(Anim{Gun: true, Hold: weapon.HoldMinigun, Aim: 1}, 0.5)
	if !l.Gun || l.Hold != weapon.HoldMinigun || l.Aim != 0.5 {
		t.Errorf("Lerp: %+v", l)
	}
}

// TestThirdPersonAnimations: aiming raises the default hold's arms
// (animateGunAim), the minigun turns the body and ignores aim, and a
// sneaking gun hold keeps TACZ's arm pitch (applied after setupAnim's
// sneak).
func TestThirdPersonAnimations(t *testing.T) {
	b := NewBody()
	hand := func(a Anim) mgl32.Vec3 { return b.Hand(b.Skin(a, nil)) }
	hold := hand(Anim{Gun: true})
	if aim := hand(Anim{Gun: true, Aim: 1}); aim.Sub(hold).Len() < 0.01 {
		t.Errorf("aim hand %v = hold hand %v", aim, hold)
	}
	mini := Anim{Gun: true, Hold: weapon.HoldMinigun}
	if h := hand(mini); h.Sub(hold).Len() < 0.05 {
		t.Errorf("minigun hand %v = default hold %v", h, hold)
	}
	aimed := mini
	aimed.Aim = 1
	if hand(aimed) != hand(mini) {
		t.Error("minigun hold changes with aim")
	}
	b.Skin(Anim{Gun: true, Sneak: true}, nil)
	if got := b.pose.Parts[partRightArm].Rot[0]; got != gunHoldArmPitch {
		t.Errorf("sneaking gun hold arm pitch %v, want %v", got, gunHoldArmPitch)
	}
}
