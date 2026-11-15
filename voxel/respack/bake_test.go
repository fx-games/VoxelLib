package respack

import (
	"os"
	"testing"

	"github.com/memmaker/voxellib/resloc"
)

// TestBake: a baked pack resolves every state as the source pack does.
func TestBake(t *testing.T) {
	src := stateTestPack()
	dst := t.TempDir()
	if _, err := src.Bake(dst); err != nil {
		t.Fatal(err)
	}
	baked := FromFS(os.DirFS(dst))
	for _, c := range []struct {
		name  string
		props map[string]string
	}{
		{"rock_slab", map[string]string{"type": "bottom"}},
		{"rock_stairs", map[string]string{"facing": "south", "shape": "straight"}},
		{"post", map[string]string{"north": "true", "east": "true"}},
	} {
		a, okA := src.Resolver()(resloc.Default+":"+c.name, c.props)
		b, okB := baked.Resolver()(resloc.Default+":"+c.name, c.props)
		if okA != okB || a.Shape != b.Shape || len(a.Boxes) != len(b.Boxes) || a.Tex != b.Tex {
			t.Errorf("%s: source %+v, baked %+v", c.name, a, b)
		}
	}
}
