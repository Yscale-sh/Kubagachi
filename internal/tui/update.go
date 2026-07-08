package tui

import (
	"context"
	"fmt"
	"os/exec"
	"strings"

	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/yscale-sh/kubagachi/internal/state"
)

// Update satisfies tea.Model and routes every incoming message.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		m.ready = true
		m.recomputeLayout()
		return m, nil

	case tickMsg:
		m.tick++
		if m.statusMsg != "" && m.tick-m.statusMsgTick > 6 {
			m.statusMsg = ""
			m.recomputeLayout()
		}
		return m, tickCmd()

	case snapshotMsg:
		m.cluster = state.ClusterState(msg)
		m.clampSelection()
		m.refreshTable()
		m.refreshEvents()
		return m, waitForSnapshot(m.snapshots)

	case textResultMsg:
		if msg.err != nil {
			m.setStatus(msg.title + ": " + msg.err.Error())
			return m, nil
		}
		m.textTitle = msg.title
		m.textBody = msg.body
		if m.mode != viewText {
			m.prevMode = m.mode
		}
		m.mode = viewText
		m.recomputeLayout()
		m.reader.SetContent(msg.body)
		m.reader.GotoBottom()
		return m, nil

	case actionDoneMsg:
		if msg.err != nil {
			m.setStatus(msg.what + " failed: " + msg.err.Error())
		} else {
			m.setStatus(msg.what + " ✓")
		}
		return m, nil

	case execFinishedMsg:
		if msg.err != nil {
			m.setStatus("shell exited: " + msg.err.Error())
		} else {
			m.setStatus("shell closed — welcome back")
		}
		return m, nil

	case tea.KeyMsg:
		return m.handleKey(msg)
	}
	return m, nil
}

func (m Model) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	// Delete confirmation intercepts everything until resolved.
	if m.confirmKey != "" {
		switch msg.String() {
		case "y", "Y", "enter":
			key := m.confirmKey
			m.confirmKey = ""
			return m, m.deletePodCmd(key)
		default:
			m.confirmKey = ""
			m.setStatus("delete cancelled")
			return m, nil
		}
	}

	// While searching, the text input captures most keys.
	if m.searching {
		switch msg.String() {
		case "esc":
			m.searching = false
			m.search.Blur()
			return m, nil
		case "enter":
			m.searching = false
			m.search.Blur()
			m.filter = m.search.Value()
			m.clampSelection()
			m.refreshTable()
			return m, nil
		}
		var cmd tea.Cmd
		m.search, cmd = m.search.Update(msg)
		m.filter = m.search.Value()
		m.clampSelection()
		m.refreshTable()
		return m, cmd
	}

	// Command mode (k9s-style `:`).
	if m.cmdMode {
		switch msg.String() {
		case "esc":
			m.cmdMode = false
			m.cmd.Blur()
			return m, nil
		case "enter":
			m.cmdMode = false
			m.cmd.Blur()
			line := strings.TrimSpace(m.cmd.Value())
			m.cmd.SetValue("")
			return m.runCommand(line)
		}
		var cmd tea.Cmd
		m.cmd, cmd = m.cmd.Update(msg)
		return m, cmd
	}

	if m.showHelp {
		if msg.String() == "ctrl+c" {
			return m, tea.Quit
		}
		if key.Matches(msg, m.keys.Help, m.keys.Esc, m.keys.Quit) {
			m.showHelp = false
		}
		return m, nil
	}

	// Text reader (logs / describe) has its own minimal keymap.
	if m.mode == viewText {
		switch {
		case key.Matches(msg, m.keys.Quit), key.Matches(msg, m.keys.Esc):
			m.mode = m.prevMode
			m.recomputeLayout()
			return m, nil
		case msg.String() == "r":
			// Re-fetch logs for the same pod.
			if strings.HasPrefix(m.textTitle, "logs ") {
				if pod, ok := m.selectedPod(); ok {
					return m, m.logsCmd(pod)
				}
			}
			return m, nil
		}
		var cmd tea.Cmd
		m.reader, cmd = m.reader.Update(msg)
		return m, cmd
	}

	// Flux view keymap: navigation plus reconcile/suspend.
	if m.mode == viewFlux {
		switch {
		case key.Matches(msg, m.keys.Quit):
			return m, tea.Quit
		case key.Matches(msg, m.keys.Esc):
			m.mode = viewHabitat
			return m, nil
		case key.Matches(msg, m.keys.Help):
			m.showHelp = true
			return m, nil
		case key.Matches(msg, m.keys.Command):
			m.cmdMode = true
			return m, m.cmd.Focus()
		case key.Matches(msg, m.keys.Up):
			if m.fluxSelected > 0 {
				m.fluxSelected--
			}
			return m, nil
		case key.Matches(msg, m.keys.Down):
			if m.fluxSelected < len(m.cluster.Flux)-1 {
				m.fluxSelected++
			}
			return m, nil
		case key.Matches(msg, m.keys.Reconcile):
			if fx, ok := m.selectedFlux(); ok {
				return m, m.fluxReconcileCmd(fx)
			}
			return m, nil
		case key.Matches(msg, m.keys.Suspend):
			if fx, ok := m.selectedFlux(); ok {
				return m, m.fluxSuspendCmd(fx)
			}
			return m, nil
		case key.Matches(msg, m.keys.Enter):
			if fx, ok := m.selectedFlux(); ok && fx.Message != "" {
				m.setStatus(fx.Message)
			}
			return m, nil
		case key.Matches(msg, m.keys.ViewToggle), key.Matches(msg, m.keys.Flux):
			m.mode = viewHabitat
			return m, nil
		}
		return m, nil
	}

	// Habitat / table views.
	switch {
	case key.Matches(msg, m.keys.Quit):
		return m, tea.Quit

	case key.Matches(msg, m.keys.Help):
		m.showHelp = true
		return m, nil

	case key.Matches(msg, m.keys.Command):
		m.cmdMode = true
		return m, m.cmd.Focus()

	case key.Matches(msg, m.keys.Search):
		m.searching = true
		m.search.SetValue(m.filter)
		m.search.CursorEnd()
		return m, m.search.Focus()

	case key.Matches(msg, m.keys.Esc):
		if m.filter != "" {
			m.filter = ""
			m.search.SetValue("")
			m.clampSelection()
			m.refreshTable()
		} else if m.nsFilter != "" {
			m.nsFilter = ""
			m.clampSelection()
			m.refreshTable()
			m.setStatus("namespace filter cleared")
		} else if m.focus != focusMain {
			m.focus = focusMain
		}
		return m, nil

	case key.Matches(msg, m.keys.Tab):
		m.focus = (m.focus + 1) % 3
		return m, nil

	case key.Matches(msg, m.keys.Events):
		m.focus = focusEvents
		return m, nil

	case key.Matches(msg, m.keys.Flux):
		if !m.cluster.FluxInstalled && len(m.cluster.Flux) == 0 {
			m.setStatus("flux: no toolkit CRDs found on this cluster")
			return m, nil
		}
		m.mode = viewFlux
		return m, nil

	case key.Matches(msg, m.keys.Enter):
		if _, ok := m.selectedPod(); ok {
			m.focus = focusDetails
		}
		return m, nil

	case key.Matches(msg, m.keys.ViewToggle):
		if m.mode == viewHabitat {
			m.mode = viewTable
		} else {
			m.mode = viewHabitat
		}
		m.refreshTable()
		return m, nil

	case key.Matches(msg, m.keys.Logs):
		if pod, ok := m.selectedPod(); ok {
			m.setStatus("fetching logs for " + pod.Key() + "…")
			return m, m.logsCmd(pod)
		}
		m.setStatus("logs: select a pod first")
		return m, nil

	case key.Matches(msg, m.keys.Describe):
		if pod, ok := m.selectedPod(); ok {
			m.setStatus("describing " + pod.Key() + "…")
			return m, m.describeCmd(pod)
		}
		m.setStatus("describe: select a pod first")
		return m, nil

	case key.Matches(msg, m.keys.Shell):
		return m.shellInto()

	case key.Matches(msg, m.keys.Delete):
		if pod, ok := m.selectedPod(); ok {
			m.confirmKey = pod.Key()
		}
		return m, nil

	case key.Matches(msg, m.keys.Up), key.Matches(msg, m.keys.Left):
		m.move(-1)
		return m, nil

	case key.Matches(msg, m.keys.Down), key.Matches(msg, m.keys.Right):
		m.move(1)
		return m, nil
	}
	return m, nil
}

// runCommand executes a `:` command line.
func (m Model) runCommand(line string) (tea.Model, tea.Cmd) {
	if line == "" {
		return m, nil
	}
	fields := strings.Fields(line)
	cmd, args := strings.ToLower(fields[0]), fields[1:]
	switch cmd {
	case "q", "quit", "exit":
		return m, tea.Quit
	case "pods", "po", "table":
		m.mode = viewTable
		m.refreshTable()
	case "habitat", "hab", "home":
		m.mode = viewHabitat
	case "flux", "ks", "kustomizations", "hr", "helmreleases", "gitops":
		if !m.cluster.FluxInstalled && len(m.cluster.Flux) == 0 {
			m.setStatus("flux: no toolkit CRDs found on this cluster")
		} else {
			m.mode = viewFlux
		}
	case "events", "ev":
		m.focus = focusEvents
	case "ns", "namespace":
		if len(args) == 0 {
			m.nsFilter = ""
			m.setStatus("namespace filter cleared")
		} else {
			m.nsFilter = args[0]
			m.setStatus("namespace → " + args[0])
		}
		m.clampSelection()
		m.refreshTable()
	case "all", "a":
		m.nsFilter = ""
		m.filter = ""
		m.search.SetValue("")
		m.clampSelection()
		m.refreshTable()
		m.setStatus("filters cleared")
	case "help", "h", "?":
		m.showHelp = true
	default:
		m.setStatus("unknown command: " + cmd + " (try pods, habitat, flux, ns, quit)")
	}
	return m, nil
}

// shellInto suspends the TUI and hands the terminal to `kubectl exec`.
func (m Model) shellInto() (tea.Model, tea.Cmd) {
	pod, ok := m.selectedPod()
	if !ok {
		m.setStatus("shell: select a pod first")
		return m, nil
	}
	if m.actions == nil {
		m.setStatus("shell: no cluster actions available")
		return m, nil
	}
	container := ""
	if len(pod.Containers) > 0 {
		container = pod.Containers[0].Name
	}
	argv := m.actions.ExecArgs(pod.Namespace, pod.Name, container)
	if len(argv) == 0 {
		m.setStatus("shell: not available in " + m.sourceName + " mode")
		return m, nil
	}
	if _, err := exec.LookPath(argv[0]); err != nil {
		m.setStatus("shell: " + argv[0] + " not found on PATH")
		return m, nil
	}
	c := exec.Command(argv[0], argv[1:]...)
	return m, tea.ExecProcess(c, func(err error) tea.Msg {
		return execFinishedMsg{err: err}
	})
}

// --- async action commands ---------------------------------------------------

func (m Model) logsCmd(pod state.PodView) tea.Cmd {
	actions := m.actions
	return func() tea.Msg {
		title := "logs " + pod.Key()
		if actions == nil {
			return textResultMsg{title: title, err: fmt.Errorf("no cluster actions available")}
		}
		container := ""
		if len(pod.Containers) > 0 {
			container = pod.Containers[0].Name
		}
		ctx, cancel := context.WithTimeout(context.Background(), actionTimeout)
		defer cancel()
		body, err := actions.Logs(ctx, pod.Namespace, pod.Name, container, 200)
		if err == nil && strings.TrimSpace(body) == "" {
			body = "(no log output yet)"
		}
		return textResultMsg{title: title, body: body, err: err}
	}
}

func (m Model) describeCmd(pod state.PodView) tea.Cmd {
	actions := m.actions
	return func() tea.Msg {
		title := "describe " + pod.Key()
		if actions == nil {
			return textResultMsg{title: title, err: fmt.Errorf("no cluster actions available")}
		}
		ctx, cancel := context.WithTimeout(context.Background(), actionTimeout)
		defer cancel()
		body, err := actions.Describe(ctx, pod.Namespace, pod.Name)
		return textResultMsg{title: title, body: body, err: err}
	}
}

func (m Model) deletePodCmd(key string) tea.Cmd {
	actions := m.actions
	parts := strings.SplitN(key, "/", 2)
	if len(parts) != 2 {
		return nil
	}
	ns, name := parts[0], parts[1]
	return func() tea.Msg {
		what := "delete " + key
		if actions == nil {
			return actionDoneMsg{what: what, err: fmt.Errorf("no cluster actions available")}
		}
		ctx, cancel := context.WithTimeout(context.Background(), actionTimeout)
		defer cancel()
		return actionDoneMsg{what: what, err: actions.DeletePod(ctx, ns, name)}
	}
}

func (m Model) fluxReconcileCmd(fx state.FluxView) tea.Cmd {
	actions := m.actions
	return func() tea.Msg {
		what := "reconcile " + fx.Kind + "/" + fx.Name
		if actions == nil {
			return actionDoneMsg{what: what, err: fmt.Errorf("no cluster actions available")}
		}
		ctx, cancel := context.WithTimeout(context.Background(), actionTimeout)
		defer cancel()
		return actionDoneMsg{what: what, err: actions.FluxReconcile(ctx, fx.Kind, fx.Namespace, fx.Name)}
	}
}

func (m Model) fluxSuspendCmd(fx state.FluxView) tea.Cmd {
	actions := m.actions
	suspend := !fx.Suspended
	verb := "suspend"
	if !suspend {
		verb = "resume"
	}
	return func() tea.Msg {
		what := verb + " " + fx.Kind + "/" + fx.Name
		if actions == nil {
			return actionDoneMsg{what: what, err: fmt.Errorf("no cluster actions available")}
		}
		ctx, cancel := context.WithTimeout(context.Background(), actionTimeout)
		defer cancel()
		return actionDoneMsg{what: what, err: actions.FluxSuspend(ctx, fx.Kind, fx.Namespace, fx.Name, suspend)}
	}
}

// move shifts the selection, or scrolls the event feed when it has focus.
func (m *Model) move(delta int) {
	if m.focus == focusEvents {
		if delta < 0 {
			m.events.ScrollUp(1)
		} else {
			m.events.ScrollDown(1)
		}
		return
	}
	n := len(m.visiblePods())
	if n == 0 {
		return
	}
	m.selected = (m.selected + delta + n) % n
	if m.mode == viewTable {
		m.podTable.SetCursor(m.selected)
	}
}

// recomputeLayout recalculates region sizes and resizes embedded components.
func (m *Model) recomputeLayout() {
	if !m.ready {
		return
	}
	m.lay = computeLayout(m.width, m.height, m.statusMsg != "")

	ew, eh := m.lay.eventsContentSize()
	m.events.Width = ew
	m.events.Height = clampPos(eh - 1) // one line reserved for the pane title

	m.search.Width = clampPos(m.width - 8)
	m.cmd.Width = clampPos(m.width - 8)

	mw, mh := m.lay.mainContentSize()
	m.podTable.SetWidth(mw)
	m.podTable.SetHeight(clampPos(mh - 1))
	m.podTable.SetColumns(tableColumns(mw))

	// Text reader fills the whole mid region (no details pane).
	m.reader.Width = clampPos(m.width - 4)
	m.reader.Height = clampPos(m.lay.midH + m.lay.eventsH - 3)

	m.refreshTable()
	m.refreshEvents()
}
