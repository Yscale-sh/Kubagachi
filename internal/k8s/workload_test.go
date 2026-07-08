package k8s

import (
	"testing"

	"github.com/yscale-sh/kubagachi/internal/state"
)

func TestYscaleWorkloadAnim(t *testing.T) {
	cases := []struct {
		ns, owner, pod, want string
	}{
		{"default", "media-transcode-burst", "", "bursting"},
		{"yscale-burst", "yscale-library-fs", "", "bursting"}, // namespace match
		{"default", "yscale-autoscaler", "", "scaling"},
		{"default", "web", "web-scale-7c9", "scaling"},
		{"default", "gpu-inference", "", "gpu-workload"},
		{"default", "edge-gateway", "", "edge-fleet"},
		{"default", "node-drainer", "", "draining"},
		{"default", "BurstPool", "", "bursting"}, // case-insensitive
		{"monitoring", "grafana", "grafana-0", ""},
		// The brand "yscale" must NOT trigger "scale" — these are plain services.
		{"yscale-media-dev-api", "yscale-media-api", "", ""},
		{"yscale", "yscale-agent-yscale-agent", "", ""},
		{"yscale-dlna", "yscale-dlna", "", ""},
		{"", "", "", ""},
	}
	for _, c := range cases {
		if got := yscaleWorkloadAnim(c.ns, c.owner, c.pod); got != c.want {
			t.Errorf("yscaleWorkloadAnim(%q,%q,%q) = %q, want %q", c.ns, c.owner, c.pod, got, c.want)
		}
	}
}

// projectMascot resolves a pod's reserved critter by namespace/owner keyword.
// Vendor services (postgres, redis) must supersede the broad Nori family so a
// database keeps its own mascot even inside a yscale/kubagachi namespace.
func TestProjectMascotPrecedence(t *testing.T) {
	cases := []struct{ ns, owner, want string }{
		{"database", "postgres", "postgres"},
		{"prod", "postgres-primary-0", "postgres"},
		// vendor supersedes the yscale/kubagachi family:
		{"yscale-prod", "postgres-0", "postgres"},
		{"kubagachi-dev-app", "postgres", "postgres"},
		{"cache", "redis-master-0", "redis"},
		{"yscale-media", "redis-0", "redis"},
		{"tools", "cartogopher-api", "cartogopher"},
		// the Yscale agent gets the phoenix, superseding Nori's broad "yscale":
		{"yscale", "yscale-agent-yscale-agent-f4c97b675", "phoenix"},
		{"yscale", "yscale-agent-yscale-agent-gateway-869c9947fd", "phoenix"},
		// the rest of the yscale family (no vendor/agent keyword) → Nori:
		{"kubagachi-dev-app", "kubagachi", "nori"},
		{"yscale-media-dev", "yscale-media-api", "nori"},
		{"yscale-website-dev", "yscale-website-api", "nori"},
		// unrelated workload → no reserved mascot (draws from the general pool):
		{"default", "checkout-api", ""},
	}
	for _, c := range cases {
		if got := projectMascot(c.ns, c.owner); got != c.want {
			t.Errorf("projectMascot(%q,%q) = %q, want %q", c.ns, c.owner, got, c.want)
		}
	}
}

// Every reserved mascot must stay out of the general pool, or it would leak
// onto unrelated pods (the exact "postgres on non-postgres pods" bug).
func TestReservedMascotsCoverVendors(t *testing.T) {
	got := map[string]bool{}
	for _, m := range reservedMascots {
		got[m] = true
	}
	for _, want := range []string{"postgres", "redis", "cartogopher", "phoenix", "nori"} {
		if !got[want] {
			t.Errorf("reservedMascots missing %q — it would leak into the general pool", want)
		}
	}
}

// Without a pixel sprite set loaded, project mascots aren't in the active set,
// so assignment falls back to the general pool and the overlay is a no-op.
func TestAssignCritterFallsBackWithoutMascots(t *testing.T) {
	// No nori/cartogopher in the built-in ASCII set, so assignCritter must
	// return some built-in critter, never panic.
	got := assignCritter("yscale-media-dev-api", "yscale-media-api", "yscale/yscale-media-api")
	if got == "" {
		t.Fatal("assignCritter returned empty")
	}
}

func TestApplyWorkloadAnimationOnlyNori(t *testing.T) {
	// Non-nori pod: untouched.
	pv := state.PodView{Owner: "media-burst", Name: "x", Status: state.StatusRunning, CritterState: state.StatusRunning, Critter: "snail"}
	applyWorkloadAnimation(&pv)
	if pv.CritterState != state.StatusRunning {
		t.Errorf("non-nori pod should be untouched, got state=%q", pv.CritterState)
	}

	// Nori + healthy + burst keyword: plays bursting.
	pv = state.PodView{Namespace: "yscale-burst", Owner: "lib", Name: "lib-0", Status: state.StatusRunning, CritterState: state.StatusRunning, Critter: "nori"}
	applyWorkloadAnimation(&pv)
	if pv.CritterState != "bursting" {
		t.Errorf("healthy nori burst pod should play bursting, got %q", pv.CritterState)
	}

	// Nori + unhealthy: keeps the health state.
	pv = state.PodView{Namespace: "yscale-burst", Owner: "lib", Name: "lib-0", Status: state.StatusCrashLoop, CritterState: state.StatusCrashLoop, Critter: "nori"}
	applyWorkloadAnimation(&pv)
	if pv.CritterState != state.StatusCrashLoop {
		t.Errorf("unhealthy nori pod should keep crashloop, got %q", pv.CritterState)
	}
}
