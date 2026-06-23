// Package critters maps pods to stable ASCII animals and renders their
// animated frames based on pod health state.
package critters

import "hash/fnv"

// Names lists every available critter type. When a pixel sprite set has
// been loaded via LoadPixelSprites, its critters are returned (sorted by
// name). Otherwise the built-in ASCII critters are returned in registry
// order.
func Names() []string {
	pixelMu.RLock()
	if pixelNames != nil {
		out := make([]string, len(pixelNames))
		copy(out, pixelNames)
		pixelMu.RUnlock()
		return out
	}
	pixelMu.RUnlock()
	out := make([]string, len(specs))
	for i, s := range specs {
		out[i] = s.name
	}
	return out
}

// Has reports whether a critter with the given name exists in the active set
// (the loaded pixel sprites if any, otherwise the built-in ASCII critters).
func Has(name string) bool {
	pixelMu.RLock()
	defer pixelMu.RUnlock()
	if pixelNames != nil {
		for _, n := range pixelNames {
			if n == name {
				return true
			}
		}
		return false
	}
	for _, s := range specs {
		if s.name == name {
			return true
		}
	}
	return false
}

// Assign deterministically picks a critter for a pod. The same hash key
// always yields the same animal, so a pod keeps its identity across refreshes
// even as its health state (and therefore its frames) changes.
//
// Callers should build the key as:
//
//	namespace + "/" + ownerName   // when the pod has a controller owner
//	namespace + "/" + podName     // otherwise
//
// Using the owner keeps every replica of a Deployment as the same animal.
// If a pixel sprite set has been loaded, the assignment picks from it
// instead of the built-in ASCII critters.
func Assign(key string) string {
	h := fnv.New32a()
	_, _ = h.Write([]byte(key))
	sum := int(h.Sum32())
	pixelMu.RLock()
	if pixelNames != nil {
		name := pixelNames[sum%len(pixelNames)]
		pixelMu.RUnlock()
		return name
	}
	pixelMu.RUnlock()
	return specs[sum%len(specs)].name
}

// AssignExcept is like Assign but never returns any of the excluded critters.
// It's used to keep project mascots (e.g. Nori, Cartogopher) out of the general
// pool so they only appear on their own workloads. Falls back to Assign if
// excluding would leave no candidates.
func AssignExcept(key string, exclude ...string) string {
	skip := make(map[string]bool, len(exclude))
	for _, e := range exclude {
		skip[e] = true
	}

	pixelMu.RLock()
	var pool []string
	if pixelNames != nil {
		for _, n := range pixelNames {
			if !skip[n] {
				pool = append(pool, n)
			}
		}
	} else {
		for _, s := range specs {
			if !skip[s.name] {
				pool = append(pool, s.name)
			}
		}
	}
	pixelMu.RUnlock()

	if len(pool) == 0 {
		return Assign(key)
	}
	h := fnv.New32a()
	_, _ = h.Write([]byte(key))
	return pool[int(h.Sum32())%len(pool)]
}
