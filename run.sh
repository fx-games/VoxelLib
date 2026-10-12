#!/bin/sh
# Build and run the explorer. Generates the texture pack and item sprites on
# first run if their sources are in ~/Downloads. Extra args go to the explorer:
#   ./run.sh -gen flat -world ''      ./run.sh -night -fly
set -e
cd "$(dirname "$0")"
if [ ! -d assets/rpg16pack ]; then go run ./cmd/rpg16pack; fi
if [ ! -d assets/items ]; then go run ./cmd/itemsheet || echo "no item sheet; running without items"; fi
go build -o explorer ./cmd/explorer
exec ./explorer "$@"
