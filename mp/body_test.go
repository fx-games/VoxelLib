package mp

import "testing"

// TestGunHold poses the arms as TACZ's default third-person hold with a gun
// in hand, and the hold survives Lerp (the server's lag compensation).
func TestGunHold(t *testing.T) {
	b := NewBody()
	bow := b.Hand(b.Skin(Anim{}, nil))
	gun := b.Hand(b.Skin(Anim{Gun: true}.Lerp(Anim{Gun: true}, 0.5), nil))
	if bow.Sub(gun).Len() < 0.05 {
		t.Fatalf("gun hold hand %v = crossbow hold %v", gun, bow)
	}
	if !(Anim{}).Lerp(Anim{Gun: true}, 0.5).Gun {
		t.Error("Lerp dropped Gun")
	}
}
