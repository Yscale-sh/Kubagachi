package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/jakenesler/kubagachi/internal/state"
)

func fluxSnapshot() state.ClusterState {
	cs := state.ClusterState{
		ClusterName:   "test",
		FluxInstalled: true,
		Nodes:         []state.NodeView{{Name: "n1", Ready: true, CPUText: "4 cpu", MemoryText: "8GiB"}},
		Pods: []state.PodView{{
			Name: "web-1", Namespace: "default", NodeName: "n1",
			Status: state.StatusRunning, CritterState: state.StatusRunning,
			Critter: "cat", Age: "1m",
		}},
		Flux: []state.FluxView{
			{Kind: "Kustomization", Name: "apps", Namespace: "flux-system",
				Ready: "False", Revision: "main@abc123", Source: "GitRepository/platform",
				Message: "kustomize build failed", Age: "3d"},
			{Kind: "HelmRelease", Name: "grafana", Namespace: "monitoring",
				Ready: "True", Suspended: true, Revision: "7.3.0", Age: "9d"},
		},
	}
	cs.Rebuild()
	return cs
}

func TestFluxViewRenders(t *testing.T) {
	ch := make(chan state.ClusterState, 1)
	var m tea.Model = New(ch, "test", nil)
	m, _ = m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m, _ = m.Update(snapshotMsg(fluxSnapshot()))

	// Enter the flux view via the `f` key.
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'f'}})
	out := m.View()
	for _, want := range []string{"flux", "Kustomization", "apps", "grafana", "⏸"} {
		if !strings.Contains(out, want) {
			t.Fatalf("flux view missing %q", want)
		}
	}

	// j moves the selection; esc leaves the view.
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if !strings.Contains(m.View(), "habitat") {
		t.Fatal("esc should return to the habitat view")
	}
}

func TestCommandModeSwitchesViews(t *testing.T) {
	ch := make(chan state.ClusterState, 1)
	var m tea.Model = New(ch, "test", nil)
	m, _ = m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m, _ = m.Update(snapshotMsg(fluxSnapshot()))

	// `:` opens command mode, "flux" + enter switches view.
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{':'}})
	for _, r := range "flux" {
		m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
	}
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if !strings.Contains(m.View(), "Kustomization") {
		t.Fatal(":flux should open the flux view")
	}

	// :ns filters by namespace.
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{':'}})
	for _, r := range "ns kube-system" {
		m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
	}
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if !strings.Contains(m.View(), "no pods match") {
		t.Fatal(":ns should filter pods to the namespace")
	}
}

func TestDeleteConfirmOverlay(t *testing.T) {
	ch := make(chan state.ClusterState, 1)
	var m tea.Model = New(ch, "test", nil)
	m, _ = m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m, _ = m.Update(snapshotMsg(fluxSnapshot()))

	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyCtrlD})
	if !strings.Contains(m.View(), "release pod") {
		t.Fatal("ctrl+d should show the delete confirm overlay")
	}
	// Any non-y key cancels.
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'n'}})
	if strings.Contains(m.View(), "release pod") {
		t.Fatal("cancel should dismiss the confirm overlay")
	}
}
