package tui

import (
	"github.com/charmbracelet/lipgloss"

	"github.com/jakenesler/kubagachi/internal/state"
)

// Yscale palette. Warm near-black, bone text, brand gold accent. Truecolor
// hexes — termenv downsamples gracefully on 256-color terminals.
var (
	colHair  = lipgloss.Color("#2a2722") // hairline borders
	colHair2 = lipgloss.Color("#3a362e") // stronger hairline
	colText  = lipgloss.Color("#e8e6e0") // bone
	colMuted = lipgloss.Color("#8a857a") // warm gray
	colFaint = lipgloss.Color("#55514a") // faint warm gray

	colGold   = lipgloss.Color("#c9b88a") // brand accent
	colGoldHi = lipgloss.Color("#d8c89a")
	colBlack  = lipgloss.Color("#0a0a0a")

	colGreen  = lipgloss.Color("#7eb87e") // running / ready
	colAmber  = lipgloss.Color("#d4b46a") // pending
	colRed    = lipgloss.Color("#d88a8a") // crash / failed / oom
	colOrange = lipgloss.Color("#d8a87e") // backoff / imagepull
	colSage   = lipgloss.Color("#9ec79a") // completed
	colGray   = lipgloss.Color("#6f6b63") // terminating
	colDim    = lipgloss.Color("#4a463f") // unknown
)

// statusColor maps a normalized pod status to its display color.
func statusColor(status string) lipgloss.Color {
	switch status {
	case state.StatusRunning:
		return colGreen
	case state.StatusPending:
		return colAmber
	case state.StatusCrashLoop, state.StatusFailed, state.StatusOOMKilled:
		return colRed
	case state.StatusBackOff, state.StatusImagePull:
		return colOrange
	case state.StatusTerminating:
		return colGray
	case state.StatusCompleted:
		return colSage
	default:
		return colDim
	}
}

type styles struct {
	headerTitle lipgloss.Style
	headerBrand lipgloss.Style
	headerMeta  lipgloss.Style
	footer      lipgloss.Style
	footerKey   lipgloss.Style
	statusLine  lipgloss.Style
	cmdPrompt   lipgloss.Style

	paneActive   lipgloss.Style
	paneInactive lipgloss.Style
	paneTitle    lipgloss.Style

	nodeBox       lipgloss.Style
	nodeTitle     lipgloss.Style
	nodeTitleDown lipgloss.Style
	podCard       lipgloss.Style
	podCardActive lipgloss.Style
	detailKey     lipgloss.Style
	detailValue   lipgloss.Style
	helpKey       lipgloss.Style
	helpDesc      lipgloss.Style
	emptyHint     lipgloss.Style
	confirmBox    lipgloss.Style
}

func newStyles() styles {
	return styles{
		headerTitle: lipgloss.NewStyle().
			Bold(true).
			Foreground(colGold),
		headerBrand: lipgloss.NewStyle().
			Foreground(colFaint),
		headerMeta: lipgloss.NewStyle().
			Foreground(colMuted),
		footer: lipgloss.NewStyle().
			Foreground(colFaint).
			Padding(0, 1),
		footerKey: lipgloss.NewStyle().
			Foreground(colGold),
		statusLine: lipgloss.NewStyle().
			Foreground(colGoldHi).
			Padding(0, 1),
		cmdPrompt: lipgloss.NewStyle().
			Foreground(colGold).
			Padding(0, 1),

		paneActive: lipgloss.NewStyle().
			Border(lipgloss.NormalBorder()).
			BorderForeground(colGold),
		paneInactive: lipgloss.NewStyle().
			Border(lipgloss.NormalBorder()).
			BorderForeground(colHair),
		paneTitle: lipgloss.NewStyle().
			Foreground(colMuted),

		nodeBox: lipgloss.NewStyle().
			Border(lipgloss.NormalBorder()).
			BorderForeground(colHair2).
			Padding(0, 1),
		nodeTitle: lipgloss.NewStyle().
			Foreground(colGreen),
		nodeTitleDown: lipgloss.NewStyle().
			Bold(true).
			Foreground(colRed),
		podCard: lipgloss.NewStyle().
			Border(lipgloss.NormalBorder()).
			BorderForeground(colHair).
			Padding(0, 1),
		podCardActive: lipgloss.NewStyle().
			Border(lipgloss.ThickBorder()).
			BorderForeground(colGold).
			Padding(0, 1),
		detailKey: lipgloss.NewStyle().
			Foreground(colMuted),
		detailValue: lipgloss.NewStyle().
			Foreground(colText),
		helpKey: lipgloss.NewStyle().
			Bold(true).
			Foreground(colGold),
		helpDesc: lipgloss.NewStyle().
			Foreground(colText),
		emptyHint: lipgloss.NewStyle().
			Foreground(colFaint).
			Italic(true),
		confirmBox: lipgloss.NewStyle().
			Border(lipgloss.ThickBorder()).
			BorderForeground(colRed).
			Padding(1, 3),
	}
}
