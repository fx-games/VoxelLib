# Renames for dependent projects

VoxelLib names no game. Old → new:

| Old | New |
|---|---|
| `github.com/memmaker/voxellib/voxel/mcres` (package `mcres`) | `github.com/memmaker/voxellib/voxel/respack` (package `respack`) |
| `github.com/memmaker/voxellib/importer/mcmodel` (package `mcmodel`) | `github.com/memmaker/voxellib/importer/bbmodel` (package `bbmodel`) |
| `github.com/memmaker/voxellib/importer/mcjava` (package `mcjava`) | `github.com/memmaker/voxellib/importer/cuboid` (package `cuboid`) |
| the default namespace as a string literal | `resloc.Default` (package `github.com/memmaker/voxellib/resloc`) |
| adding the default namespace by hand | `resloc.Qualify(name)` or `voxel.Namespaced(name)` |
| explorer flag `-mcassets` | `-modelpack` |

Exported identifiers did not change apart from the package names.

## Migrating a project

From the project root, after pulling VoxelLib:

```sh
files=$(grep -rlE 'voxellib/(voxel/mcres|importer/mc(model|java))' --include='*.go' .)
perl -pi -e '
  s#voxellib/voxel/mcres#voxellib/voxel/respack#g;
  s#voxellib/importer/mcmodel#voxellib/importer/bbmodel#g;
  s#voxellib/importer/mcjava#voxellib/importer/cuboid#g;
  s/\bmcres\./respack./g; s/\bmcmodel\./bbmodel./g; s/\bmcjava\./cuboid./g' $files
gofmt -w $files && go build ./... && go vet ./...
```

A file that imported one of these packages under its own alias keeps working;
only the import path changes.

## Exceptions

- `resloc` is the only package that spells out the default namespace; it is part
  of the on-disk format of worlds and resource packs.
- Resource packs keep their on-disk layout (`assets/<namespace>/...`), e.g.
  `assets/rpg16pack`.
- Third-party credits and READMEs under `assets/` are left as their authors wrote them.
