// Package tui implements the kubagachi terminal UI on top of Bubble Tea.
package tui

import (
	"context"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/table"
	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/jakenesler/kubagachi/internal/state"
)

// animTickInterval is how often the critter animation advances. Frames are
// replaced in place — every card is a fixed-size cell, so a tick only swaps
// glyphs and never reflows the layout.
const animTickInterval = 500 * time.Millisecond

// actionTimeout bounds every one-shot cluster action (logs, describe, delete).
const actionTimeout = 15 * time.Second

// Actions is the cluster side-effect surface the TUI drives. The live source
// implements it with client-go; the demo source returns friendly stubs.
type Actions interface {
	Logs(ctx context.Context, namespace, pod, container string, tail int64) (string, error)
	Describe(ctx context.Context, namespace, pod string) (string, error)
	DeletePod(ctx context.Context, namespace, pod string) error
	// ExecArgs returns the argv for an interactive shell, or nil when shells
	// are unsupported (demo mode).
	ExecArgs(namespace, pod, container string) []string
	FluxReconcile(ctx context.Context, kind, namespace, name string) error
	FluxSuspend(ctx context.Context, kind, namespace, name string, suspend bool) error
}

type focusArea int

const (
	focusMain focusArea = iota
	focusDetails
	focusEvents
)

type viewMode int

const (
	viewHabitat viewMode = iota
	viewTable
	viewFlux
	viewText // logs / describe reader
)

// Messages flowing through the Bubble Tea runtime.
type (
	tickMsg     struct{}
	snapshotMsg state.ClusterState

	// textResultMsg carries logs/describe output into the reader view.
	textResultMsg struct {
		title string
		body  string
		err   error
	}
	// actionDoneMsg reports a fire-and-forget action (delete, reconcile…).
	actionDoneMsg struct {
		what string
		err  error
	}
	// execFinishedMsg arrives after a shell passthrough returns.
	execFinishedMsg struct{ err error }
)

// Model is the root Bubble Tea model for kubagachi.
type Model struct {
	cluster    state.ClusterState
	snapshots  <-chan state.ClusterState
	sourceName string
	actions    Actions

	keys   keyMap
	styles styles
	lay    layout

	width, height int
	tick          int
	ready         bool

	focus    focusArea
	mode     viewMode
	prevMode viewMode // view to return to when leaving the text reader
	selected int

	searching bool
	search    textinput.Model
	filter    string

	cmdMode bool
	cmd     textinput.Model

	// nsFilter narrows the visible pods to one namespace (":ns <name>").
	nsFilter string

	// confirmKey holds the pod key awaiting delete confirmation.
	confirmKey string

	showHelp      bool
	statusMsg     string
	statusMsgTick int

	fluxSelected int

	textTitle string
	textBody  string
	reader    viewport.Model

	podTable table.Model
	events   viewport.Model
}

// New constructs the root model. snapshots delivers ClusterState updates from
// either the live cluster watcher or the demo data source. sourceName is a
// short label ("cluster" or "demo") shown in the header. actions may be nil.
func New(snapshots <-chan state.ClusterState, sourceName string, actions Actions) Model {
	ti := textinput.New()
	ti.Prompt = "/ "
	ti.Placeholder = "filter by name, namespace or status"
	ti.CharLimit = 64

	cmd := textinput.New()
	cmd.Prompt = ": "
	cmd.Placeholder = "pods · habitat · flux · events · ns <name> · all · quit"
	cmd.CharLimit = 64

	tbl := table.New(table.WithFocused(true), table.WithHeight(10))
	tbl.SetColumns(tableColumns(80))
	st := table.DefaultStyles()
	st.Header = st.Header.Bold(true).Foreground(colGold)
	st.Selected = st.Selected.Bold(true).Foreground(colBlack).Background(colGold)
	tbl.SetStyles(st)

	return Model{
		snapshots:  snapshots,
		sourceName: sourceName,
		actions:    actions,
		keys:       defaultKeys(),
		styles:     newStyles(),
		search:     ti,
		cmd:        cmd,
		podTable:   tbl,
		events:     viewport.New(80, 6),
		reader:     viewport.New(80, 20),
	}
}

// Init satisfies tea.Model. It kicks off the animation ticker and the first
// blocking read from the snapshot channel.
func (m Model) Init() tea.Cmd {
	return tea.Batch(tickCmd(), waitForSnapshot(m.snapshots), textinput.Blink)
}

func tickCmd() tea.Cmd {
	return tea.Tick(animTickInterval, func(time.Time) tea.Msg { return tickMsg{} })
}

// waitForSnapshot blocks on the snapshot channel and yields the next update
// as a message. It is re-issued after every snapshot so updates keep flowing.
func waitForSnapshot(ch <-chan state.ClusterState) tea.Cmd {
	return func() tea.Msg {
		snap, ok := <-ch
		if !ok {
			return nil
		}
		return snapshotMsg(snap)
	}
}

// visiblePods returns the node-grouped flat pod list with the active search
// and namespace filters applied.
func (m Model) visiblePods() []state.PodView {
	all := m.cluster.FlatPods()
	if m.filter == "" && m.nsFilter == "" {
		return all
	}
	needle := strings.ToLower(m.filter)
	out := make([]state.PodView, 0, len(all))
	for _, p := range all {
		if m.nsFilter != "" && p.Namespace != m.nsFilter {
			continue
		}
		if needle != "" {
			hay := strings.ToLower(p.Name + " " + p.Namespace + " " + p.Status + " " + p.Reason + " " + p.NodeName)
			if !strings.Contains(hay, needle) {
				continue
			}
		}
		out = append(out, p)
	}
	return out
}

// selectedPod returns the currently highlighted pod, if any.
func (m Model) selectedPod() (state.PodView, bool) {
	pods := m.visiblePods()
	if m.selected < 0 || m.selected >= len(pods) {
		return state.PodView{}, false
	}
	return pods[m.selected], true
}

// selectedFlux returns the currently highlighted flux object, if any.
func (m Model) selectedFlux() (state.FluxView, bool) {
	if m.fluxSelected < 0 || m.fluxSelected >= len(m.cluster.Flux) {
		return state.FluxView{}, false
	}
	return m.cluster.Flux[m.fluxSelected], true
}

// clampSelection keeps the selection indexes within range.
func (m *Model) clampSelection() {
	if n := len(m.visiblePods()); n == 0 {
		m.selected = 0
	} else if m.selected >= n {
		m.selected = n - 1
	} else if m.selected < 0 {
		m.selected = 0
	}
	if n := len(m.cluster.Flux); n == 0 {
		m.fluxSelected = 0
	} else if m.fluxSelected >= n {
		m.fluxSelected = n - 1
	} else if m.fluxSelected < 0 {
		m.fluxSelected = 0
	}
}

func (m *Model) setStatus(msg string) {
	m.statusMsg = msg
	m.statusMsgTick = m.tick
}
