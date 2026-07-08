package app

import (
	"context"
	"testing"
	"time"

	"github.com/yscale-sh/kubagachi/internal/state"
)

func TestDemoBuildProducesRichCluster(t *testing.T) {
	cs := demoSource{}.build()

	if len(cs.Nodes) != 3 {
		t.Fatalf("want 3 demo nodes, got %d", len(cs.Nodes))
	}
	if len(cs.Pods) < 15 || len(cs.Pods) > 20 {
		t.Fatalf("want 15-20 demo pods, got %d", len(cs.Pods))
	}
	if len(cs.Events) == 0 {
		t.Fatal("want demo events, got none")
	}
	for name, count := range map[string]int{
		"endpoints":                  len(cs.Endpoints),
		"network policies":           len(cs.NetworkPolicies),
		"resource quotas":            len(cs.ResourceQuotas),
		"limit ranges":               len(cs.LimitRanges),
		"horizontal pod autoscalers": len(cs.HorizontalPodAutoscalers),
		"pod disruption budgets":     len(cs.PodDisruptionBudgets),
	} {
		if count == 0 {
			t.Fatalf("want demo %s, got none", name)
		}
	}
	web := toWebSnapshot(cs, "demo")
	for name, count := range map[string]int{
		"endpoints":                  len(web.Endpoints),
		"network policies":           len(web.NetworkPolicies),
		"resource quotas":            len(web.ResourceQuotas),
		"limit ranges":               len(web.LimitRanges),
		"horizontal pod autoscalers": len(web.HorizontalPodAutoscalers),
		"pod disruption budgets":     len(web.PodDisruptionBudgets),
	} {
		if count == 0 {
			t.Fatalf("want web demo %s, got none", name)
		}
	}
	if cs.Summary.Pods != len(cs.Pods) {
		t.Errorf("summary not rebuilt: Summary.Pods=%d, len(Pods)=%d", cs.Summary.Pods, len(cs.Pods))
	}

	for _, p := range cs.Pods {
		if p.Critter == "" || p.CritterState == "" {
			t.Errorf("pod %s missing critter/state: %q/%q", p.Name, p.Critter, p.CritterState)
		}
	}

	seen := map[string]bool{}
	for _, p := range cs.Pods {
		seen[p.Status] = true
	}
	for _, want := range []string{
		state.StatusRunning, state.StatusCrashLoop, state.StatusPending,
		state.StatusCompleted, state.StatusUnknown, state.StatusImagePull,
	} {
		if !seen[want] {
			t.Errorf("demo cluster missing a pod with status %q", want)
		}
	}
}

func TestDemoStreamEmitsSnapshot(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	ch, err := demoSource{}.Stream(ctx)
	if err != nil {
		t.Fatalf("Stream returned error: %v", err)
	}
	select {
	case cs := <-ch:
		if len(cs.Pods) == 0 {
			t.Fatal("first demo snapshot has no pods")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("demo source emitted no snapshot")
	}
}

func TestDemoStreamClosesOnCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	ch, err := demoSource{}.Stream(ctx)
	if err != nil {
		t.Fatalf("Stream returned error: %v", err)
	}
	<-ch // drain initial snapshot
	cancel()

	deadline := time.After(2 * time.Second)
	for {
		select {
		case _, open := <-ch:
			if !open {
				return // channel closed as expected
			}
		case <-deadline:
			t.Fatal("channel not closed after context cancel")
		}
	}
}
