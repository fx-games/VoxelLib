// Package resloc holds the default namespace of resource locations
// ("namespace:path"): ids without a namespace in worlds, packs and data files
// belong to it.
package resloc

import "strings"

// Default is the namespace of unqualified ids. It is part of the on-disk
// format of worlds and resource packs, so it cannot change.
const Default = "minecraft"

// Split returns a location's namespace (Default when it has none) and path.
func Split(loc string) (namespace, path string) {
	if ns, p, ok := strings.Cut(loc, ":"); ok {
		return ns, p
	}
	return Default, loc
}

// Qualify adds the Default namespace to a location without one.
func Qualify(loc string) string {
	ns, p := Split(loc)
	return ns + ":" + p
}
