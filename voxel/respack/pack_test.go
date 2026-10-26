package respack

import "testing"

func TestPackWithoutLadderDrawsOne(t *testing.T) {
	p, err := Open(t.TempDir())
	if err != nil {
		t.Skip(err)
	}
	f, _, err := p.TextureFrames("block/ladder")
	if err != nil || f[0].Bounds().Dx() != 16 {
		t.Fatalf("%v", err)
	}
}
