// Package basepack is the GUI every game has before it brings its own: a
// bitmap font, buttons, the menu background and English menu strings,
// laid out as a resource pack (cmd/genbasepack draws the textures). Put
// it last in a ui.StackFS so a game's pack, or a client jar, wins:
//
//	ctx, _ := ui.NewContext(canvas, ui.StackFS{mine, basepack.FS}, lang)
package basepack

import (
	"embed"
	"io/fs"
)

//go:embed assets
var files embed.FS

// FS is the pack (assets/minecraft/...).
var FS fs.FS = files
