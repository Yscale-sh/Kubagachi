package k8s

import (
	"strings"

	"github.com/yscale-sh/kubagachi/internal/critters"
	"github.com/yscale-sh/kubagachi/internal/state"
)

// projectMascots reserves a critter for a workload identity: a pod whose
// namespace or owner matches one of the keywords always gets that mascot, and
// the mascot never appears on anything else. First match wins, so entries are
// ordered most-specific-first. Vendor services (postgres, redis) come before
// the broad Nori family so a postgres pod is the elephant even inside a
// yscale/kubagachi namespace — vendor identity supersedes the brand family.
var projectMascots = []struct {
	critter  string
	keywords []string
}{
	{"postgres", []string{"postgres"}},
	{"redis", []string{"redis"}},
	{"cartogopher", []string{"cartogopher"}},
	// The Yscale phoenix is the agent's mascot specifically — it must beat Nori's
	// broad "yscale" match, so it sits above Nori with the more specific keyword.
	{"phoenix", []string{"yscale-agent"}},
	{"nori", []string{"yscale", "jak3s", "kubagachi", "kubekritter"}},
}

// reservedMascots is every reserved critter, kept out of the general pool.
var reservedMascots = func() []string {
	out := make([]string, len(projectMascots))
	for i, pm := range projectMascots {
		out[i] = pm.critter
	}
	return out
}()

// projectMascot returns the reserved critter a workload identity belongs to
// (by namespace/owner keyword), or "" when none match. Pure and order-sensitive:
// the first matching entry wins, so vendors supersede the Nori family.
func projectMascot(namespace, owner string) string {
	hay := strings.ToLower(namespace + " " + owner)
	for _, pm := range projectMascots {
		for _, kw := range pm.keywords {
			if strings.Contains(hay, kw) {
				return pm.critter
			}
		}
	}
	return ""
}

// assignCritter picks a pod's critter. A pod that belongs to a reserved mascot
// gets it (when that sprite is actually loaded); everything else draws from the
// general pool, which never includes a reserved mascot — so postgres/redis/
// Cartogopher/Nori only ever show on their own workloads.
func assignCritter(namespace, owner, key string) string {
	if m := projectMascot(namespace, owner); m != "" && critters.Has(m) {
		return m
	}
	return critters.AssignExcept(key, reservedMascots...)
}

// noriCritter is the shared Yscale-family mascot that carries the workload
// animation decks (critters/nori/sprite-sheet-bursting.png, etc.).
const noriCritter = "nori"

// yscaleWorkloadKeywords maps an owner/pod/namespace substring to the workload
// animation Nori plays. First match wins, so more specific keywords come first
// (e.g. "autoscal" before the broader "scale").
var yscaleWorkloadKeywords = []struct {
	keyword string
	anim    string
}{
	{"autoscal", "scaling"},
	{"burst", "bursting"},
	{"gpu", "gpu-workload"},
	{"edge", "edge-fleet"},
	{"drain", "draining"},
	{"scale", "scaling"},
}

// yscaleWorkloadAnim returns the workload animation implied by a pod's
// namespace, owner, or name (keyword match), or "" when none applies.
func yscaleWorkloadAnim(namespace, owner, podName string) string {
	hay := strings.ToLower(namespace + " " + owner + " " + podName)
	for _, kw := range yscaleWorkloadKeywords {
		if tokenMatch(hay, kw.keyword) {
			return kw.anim
		}
	}
	return ""
}

// tokenMatch reports whether kw appears in hay at a word boundary, so the brand
// "yscale" does NOT match the workload keyword "scale" (which would tag every
// yscale pod), while "web-scale" and "autoscaler" still do. A boundary is the
// string start or a separator/digit before the match.
func tokenMatch(hay, kw string) bool {
	for i := 0; ; {
		j := strings.Index(hay[i:], kw)
		if j < 0 {
			return false
		}
		pos := i + j
		if pos == 0 || isBoundary(hay[pos-1]) {
			return true
		}
		i = pos + 1
	}
}

func isBoundary(c byte) bool {
	switch {
	case c == '-', c == '_', c == '.', c == '/', c == ' ', c == ':':
		return true
	case c >= '0' && c <= '9':
		return true
	default:
		return false
	}
}

// applyWorkloadAnimation overlays a Yscale workload animation onto a Nori pod.
// Only Nori pods are affected (the decks live under critters/nori). A healthy
// pod plays the activity; an unhealthy one keeps its health state so problems
// stay visible — still on the Nori sprite.
func applyWorkloadAnimation(pv *state.PodView) {
	if pv.Critter != noriCritter {
		return
	}
	if pv.Status != state.StatusRunning {
		return
	}
	if anim := yscaleWorkloadAnim(pv.Namespace, pv.Owner, pv.Name); anim != "" {
		pv.CritterState = anim
	}
}
