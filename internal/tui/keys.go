package tui

import "github.com/charmbracelet/bubbles/key"

// keyMap holds every key binding used by the TUI. Bindings carry their own
// help text so the help screen and footer stay in sync automatically.
type keyMap struct {
	Up         key.Binding
	Down       key.Binding
	Left       key.Binding
	Right      key.Binding
	Tab        key.Binding
	Enter      key.Binding
	Search     key.Binding
	Command    key.Binding
	Esc        key.Binding
	ViewToggle key.Binding
	Flux       key.Binding
	Events     key.Binding
	Logs       key.Binding
	Describe   key.Binding
	Shell      key.Binding
	Delete     key.Binding
	Reconcile  key.Binding
	Suspend    key.Binding
	Help       key.Binding
	Quit       key.Binding
}

func defaultKeys() keyMap {
	return keyMap{
		Up: key.NewBinding(
			key.WithKeys("up", "k"),
			key.WithHelp("↑/k", "move up"),
		),
		Down: key.NewBinding(
			key.WithKeys("down", "j"),
			key.WithHelp("↓/j", "move down"),
		),
		Left: key.NewBinding(
			key.WithKeys("left", "h"),
			key.WithHelp("←/h", "previous"),
		),
		Right: key.NewBinding(
			key.WithKeys("right"),
			key.WithHelp("→", "next"),
		),
		Tab: key.NewBinding(
			key.WithKeys("tab"),
			key.WithHelp("tab", "switch pane"),
		),
		Enter: key.NewBinding(
			key.WithKeys("enter"),
			key.WithHelp("enter", "inspect"),
		),
		Search: key.NewBinding(
			key.WithKeys("/"),
			key.WithHelp("/", "filter"),
		),
		Command: key.NewBinding(
			key.WithKeys(":"),
			key.WithHelp(":", "command"),
		),
		Esc: key.NewBinding(
			key.WithKeys("esc"),
			key.WithHelp("esc", "back/clear"),
		),
		ViewToggle: key.NewBinding(
			key.WithKeys("v"),
			key.WithHelp("v", "habitat/table"),
		),
		Flux: key.NewBinding(
			key.WithKeys("f"),
			key.WithHelp("f", "flux"),
		),
		Events: key.NewBinding(
			key.WithKeys("e"),
			key.WithHelp("e", "events"),
		),
		Logs: key.NewBinding(
			key.WithKeys("l"),
			key.WithHelp("l", "logs"),
		),
		Describe: key.NewBinding(
			key.WithKeys("d"),
			key.WithHelp("d", "describe"),
		),
		Shell: key.NewBinding(
			key.WithKeys("s"),
			key.WithHelp("s", "shell"),
		),
		Delete: key.NewBinding(
			key.WithKeys("ctrl+d"),
			key.WithHelp("ctrl+d", "delete pod"),
		),
		Reconcile: key.NewBinding(
			key.WithKeys("r"),
			key.WithHelp("r", "reconcile (flux)"),
		),
		Suspend: key.NewBinding(
			key.WithKeys("s"),
			key.WithHelp("s", "suspend/resume (flux)"),
		),
		Help: key.NewBinding(
			key.WithKeys("?"),
			key.WithHelp("?", "help"),
		),
		Quit: key.NewBinding(
			key.WithKeys("q", "ctrl+c"),
			key.WithHelp("q", "quit"),
		),
	}
}

// helpEntries returns ordered binding/desc pairs for the full help screen.
func (k keyMap) helpEntries() []key.Binding {
	return []key.Binding{
		k.Up, k.Down, k.Left, k.Right,
		k.Tab, k.Enter, k.Search, k.Command, k.Esc,
		k.ViewToggle, k.Flux, k.Events,
		k.Logs, k.Describe, k.Shell, k.Delete,
		k.Reconcile, k.Suspend,
		k.Help, k.Quit,
	}
}
