package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/jakenesler/kubagachi/internal/state"
)

func sampleCluster() state.ClusterState {
	cs := state.ClusterState{
		ClusterName:   "test",
		AllNamespaces: true,
		Nodes: []state.NodeView{
			{Name: "node-a", Ready: true, CPUText: "4 cpu", MemoryText: "16GiB"},
			{Name: "node-b", Ready: false, CPUText: "8 cpu", MemoryText: "32GiB"},
		},
		Pods: []state.PodView{
			{Name: "web-1", Namespace: "default", NodeName: "node-a", Status: state.StatusRunning, Critter: "cat", CritterState: state.StatusRunning},
			{Name: "api-1", Namespace: "default", NodeName: "node-a", Status: state.StatusCrashLoop, Critter: "dog", CritterState: state.StatusCrashLoop, Restarts: 5},
			{Name: "job-1", Namespace: "batch", NodeName: "node-b", Status: state.StatusCompleted, Critter: "fox", CritterState: state.StatusCompleted},
			{Name: "lost-1", Namespace: "monitoring", NodeName: "", Status: state.StatusUnknown, Critter: "ghost", CritterState: state.StatusUnknown},
		},
		Events: []state.EventView{
			{Time: "1m", Type: "Warning", Reason: "BackOff", Object: "Pod/api-1", Message: "restarting"},
			{Time: "2m", Type: "Normal", Reason: "Scheduled", Object: "Pod/web-1", Message: "assigned"},
		},
	}
	cs.Rebuild()
	return cs
}

func keyMsg(s string) tea.KeyMsg {
	switch s {
	case "up":
		return tea.KeyMsg{Type: tea.KeyUp}
	case "down":
		return tea.KeyMsg{Type: tea.KeyDown}
	case "left":
		return tea.KeyMsg{Type: tea.KeyLeft}
	case "right":
		return tea.KeyMsg{Type: tea.KeyRight}
	case "tab":
		return tea.KeyMsg{Type: tea.KeyTab}
	case "enter":
		return tea.KeyMsg{Type: tea.KeyEnter}
	case "esc":
		return tea.KeyMsg{Type: tea.KeyEsc}
	default:
		return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s)}
	}
}

// TestModelLifecycle drives the model through every navigation path and
// asserts View never panics or returns an empty frame.
func TestModelLifecycle(t *testing.T) {
	ch := make(chan state.ClusterState)
	var m tea.Model = New(ch, "test", nil)

	m, _ = m.Update(tea.WindowSizeMsg{Width: 130, Height: 44})
	m, _ = m.Update(snapshotMsg(sampleCluster()))

	for i := 0; i < 8; i++ {
		m, _ = m.Update(tickMsg{})
	}

	steps := []string{
		"down", "down", "right", "up", "left", // navigation
		"tab", "tab", "tab", // focus cycling
		"v", "down", "v", // habitat <-> table
		"e", "down", "up", // event feed scroll
		"enter", "esc", // inspect / close
		"r", "n", "l", "d", // refresh + placeholders
		"?", "?", // help open/close
	}
	for _, s := range steps {
		m, _ = m.Update(keyMsg(s))
		if v := m.View(); v == "" {
			t.Fatalf("empty view after key %q", s)
		}
	}
}

func TestModelSearchFiltersPods(t *testing.T) {
	ch := make(chan state.ClusterState)
	var m tea.Model = New(ch, "test", nil)
	m, _ = m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m, _ = m.Update(snapshotMsg(sampleCluster()))

	m, _ = m.Update(keyMsg("/"))
	for _, r := range "crash" {
		m, _ = m.Update(keyMsg(string(r)))
	}
	m, _ = m.Update(keyMsg("enter"))

	mdl := m.(Model)
	pods := mdl.visiblePods()
	if len(pods) != 1 || pods[0].Status != state.StatusCrashLoop {
		t.Fatalf("search for 'crash' should match 1 crashloop pod, got %d", len(pods))
	}

	m, _ = m.Update(keyMsg("esc"))
	if got := len(m.(Model).visiblePods()); got != 4 {
		t.Fatalf("esc should clear filter, expected 4 pods, got %d", got)
	}
}

func TestModelHandlesTinyTerminal(t *testing.T) {
	ch := make(chan state.ClusterState)
	var m tea.Model = New(ch, "test", nil)
	m, _ = m.Update(tea.WindowSizeMsg{Width: 24, Height: 9})
	m, _ = m.Update(snapshotMsg(sampleCluster()))
	if v := m.View(); !strings.Contains(v, "kubagachi") {
		t.Fatalf("tiny terminal view missing app name: %q", v)
	}
}

func TestModelViewRendersExpectedContent(t *testing.T) {
	ch := make(chan state.ClusterState)
	var m tea.Model = New(ch, "demo", nil)
	m, _ = m.Update(tea.WindowSizeMsg{Width: 140, Height: 46})
	m, _ = m.Update(snapshotMsg(sampleCluster()))
	m, _ = m.Update(tickMsg{})

	v := m.View()
	for _, want := range []string{"kubagachi", "node-a", "node-b", "web-1", "habitat", "events"} {
		if !strings.Contains(v, want) {
			t.Errorf("rendered view is missing %q", want)
		}
	}

	// Table view should render the column headers.
	m, _ = m.Update(keyMsg("v"))
	if tv := m.View(); !strings.Contains(tv, "STATUS") {
		t.Error("table view missing STATUS column header")
	}
}

func TestModelQuitKey(t *testing.T) {
	ch := make(chan state.ClusterState)
	var m tea.Model = New(ch, "test", nil)
	m, _ = m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	_, cmd := m.Update(keyMsg("q"))
	if cmd == nil {
		t.Fatal("pressing q should return a quit command")
	}
}
