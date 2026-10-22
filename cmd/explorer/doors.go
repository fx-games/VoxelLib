package main

import (
	"strconv"
	"strings"

	"github.com/memmaker/voxellib/voxel"
)

// toggleDoor opens or closes the door at p, both halves; false if p is not
// a door.
func (g *game) toggleDoor(p voxel.BlockPos) bool {
	reg := g.Eng.World.Reg
	info := reg.Info(g.Eng.Block(p.X, p.Y, p.Z))
	if !strings.HasSuffix(info.Name, "_door") {
		return false
	}
	props := reg.StateProps(g.Eng.Block(p.X, p.Y, p.Z))
	open := strconv.FormatBool(props["open"] != "true")
	other := p.Y + 1
	if props["half"] == "upper" {
		other = p.Y - 1
	}
	for _, y := range []int{p.Y, other} {
		id := g.Eng.Block(p.X, y, p.Z)
		if reg.Info(id).Name != info.Name {
			continue
		}
		pr := reg.StateProps(id)
		pr["open"] = open
		g.Eng.SetBlock(p.X, y, p.Z, reg.State(info.Name, pr))
	}
	return true
}
