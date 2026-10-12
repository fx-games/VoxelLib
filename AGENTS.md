# Notes for agents

- Read ARCHITECTURE.md first. Keep the layering: `character`, `items`, `items/*` and
  `worldgen/*` must not import `voxel/engine` or `voxel/render`. GL code lives only in
  `voxel/render` and `cmd/`.
- Verify with `gofmt -l . && go vet ./... && go test ./...`.
- Visual check without touching the user's mouse or keyboard:
  `go build -o explorer ./cmd/explorer && ./explorer -world '' -shot /tmp/x.png -shot-at 160`
  (add `-pos`, `-look`, `-edits`, `-hold` to set up a scene). Look at the PNG.
- Frame-time budget: the CPU must stay under 12 ms per frame while flying at 22–30 blocks/s.
  Check with `-autofly 30 -verbose`. Never take the world write lock on the main thread
  per frame, and never block on GL read-backs.
- Generated assets (`assets/rpg16pack`, `assets/items`) are committed; regenerate them with
  the `cmd/` tools instead of editing them by hand.
- macOS: the GL context must stay on the main thread (`runtime.LockOSThread` in init).
