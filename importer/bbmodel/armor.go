package bbmodel

import (
	"strings"

	"github.com/go-gl/mathgl/mgl32"
	"github.com/memmaker/voxellib/model"
)

// armorLimb maps an armor group (lower case, "armor" stripped) to the
// canonical body part the reference game attaches it to.
var armorLimb = map[string]string{
	"head": "head", "helmet": "head",
	"body": "torso", "chest": "torso",
	"rightarm": "rightarm", "leftarm": "leftarm",
	"rightleg": "rightleg", "leftleg": "leftleg",
	"rightboot": "rightleg", "leftboot": "leftleg",
}

// attachArmor moves top-level armor groups (armorLeftBoot, ...) under the
// body part they belong to. The reference's armor renderer attaches them by
// name, so some models leave them at the root, where they would stay put
// while the limb moves. Requires bind = pure pivot translation (as both
// parsers produce) and reorders bones so parents precede children.
func attachArmor(m *model.Model) {
	limb := map[string]int{}
	for i, b := range m.Bones {
		if c := canonBone(b.Name); limb[c] == 0 {
			limb[c] = i + 1 // first match; 0 = none
		}
	}
	pivot := func(i int) mgl32.Vec3 { return m.Bones[i].InvBind.Col(3).Vec3().Mul(-1) }
	moved := false
	for i := range m.Bones {
		b := &m.Bones[i]
		n := strings.ToLower(b.Name)
		if b.Parent >= 0 || !strings.HasPrefix(n, "armor") {
			continue
		}
		p := limb[armorLimb[strings.TrimPrefix(n, "armor")]] - 1
		if p < 0 || p == i {
			continue
		}
		b.T = pivot(i).Sub(pivot(p))
		b.Parent = p
		moved = true
	}
	if moved {
		sortBones(m)
	}
}

// sortBones reorders bones depth first so every parent comes before its
// children, remapping parents and vertex joints.
func sortBones(m *model.Model) {
	kids := make([][]int, len(m.Bones))
	var roots []int
	for i, b := range m.Bones {
		if b.Parent < 0 {
			roots = append(roots, i)
		} else {
			kids[b.Parent] = append(kids[b.Parent], i)
		}
	}
	order := make([]int, 0, len(m.Bones))
	var visit func(i int)
	visit = func(i int) {
		order = append(order, i)
		for _, k := range kids[i] {
			visit(k)
		}
	}
	for _, r := range roots {
		visit(r)
	}
	newIdx := make([]int, len(m.Bones))
	for n, o := range order {
		newIdx[o] = n
	}
	bones := make([]model.Bone, len(order))
	for n, o := range order {
		bones[n] = m.Bones[o]
		if p := bones[n].Parent; p >= 0 {
			bones[n].Parent = newIdx[p]
		}
	}
	m.Bones = bones
	for mi := range m.Meshes {
		vs := m.Meshes[mi].Verts
		for vi := range vs {
			for j, w := range vs[vi].Weights {
				if w > 0 {
					vs[vi].Joints[j] = uint16(newIdx[vs[vi].Joints[j]])
				}
			}
		}
	}
}
