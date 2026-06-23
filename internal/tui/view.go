package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/table"
	"github.com/charmbracelet/lipgloss"

	"github.com/jakenesler/kubagachi/internal/critters"
	"github.com/jakenesler/kubagachi/internal/state"
)

const podCardInnerW = 18

// podCardOuterW is the full width a rendered pod card occupies: inner content
// width + horizontal padding (2) + border (2).
const podCardOuterW = podCardInnerW + 4

// centerPixelFrame pads each line of an inline-image placeholder (assumed to
// be critters.PixelTargetSize visible cells wide) with leading spaces so it
// sits centered inside an inner width of innerW.
func centerPixelFrame(frame string, innerW int) string {
	pad := (innerW - critters.PixelTargetSize) / 2
	if pad <= 0 {
		return frame
	}
	prefix := strings.Repeat(" ", pad)
	lines := strings.Split(frame, "\n")
	for i, ln := range lines {
		lines[i] = prefix + ln
	}
	return strings.Join(lines, "\n")
}

// View satisfies tea.Model and renders the whole screen. Output is always
// exactly terminal-sized: regions have fixed heights and cards have fixed
// widths, so an animation tick replaces glyphs in place — nothing scrolls.
func (m Model) View() string {
	if !m.ready || m.width < 20 || m.height < 8 {
		return "kubagachi — initializing… (resize the terminal if this persists)"
	}
	if m.showHelp {
		return m.renderHelp()
	}
	if m.confirmKey != "" {
		return m.renderConfirm()
	}

	parts := []string{m.renderHeader(), m.renderMid(), m.renderEvents()}
	if m.statusMsg != "" {
		parts = append(parts, m.styles.statusLine.Render(truncate("» "+m.statusMsg, m.width-2)))
	}
	parts = append(parts, m.renderFooter())

	out := lipgloss.JoinVertical(lipgloss.Left, parts...)
	out = lipgloss.NewStyle().MaxWidth(m.width).MaxHeight(m.height).Render(out)
	return critters.ResolveInlineImages(out)
}

func (m Model) renderHeader() string {
	s := m.styles
	ns := m.nsFilter
	if ns == "" {
		ns = m.cluster.Namespace
		if m.cluster.AllNamespaces {
			ns = "all"
		}
	}
	if ns == "" {
		ns = "default"
	}
	ctx := m.cluster.ClusterName
	if ctx == "" {
		ctx = m.sourceName
	}

	left := s.headerTitle.Render(" kubagachi") + s.headerBrand.Render(" · yscale")
	meta := s.headerMeta.Render(fmt.Sprintf("  %s · ns %s", ctx, ns))
	if m.cluster.FluxInstalled {
		meta += s.headerBrand.Render("  flux✓")
	}
	content := left + meta + "  " + renderSummary(m.cluster.Summary)
	return padLineTo(content, m.width)
}

func renderSummary(sum state.SummaryView) string {
	dot := func(c lipgloss.Color, label string, n int) string {
		if n == 0 {
			return ""
		}
		return lipgloss.NewStyle().Foreground(c).Render(fmt.Sprintf("●%d %s", n, label))
	}
	parts := []string{
		lipgloss.NewStyle().Foreground(colMuted).Render(fmt.Sprintf("%d nodes · %d pods", sum.Nodes, sum.Pods)),
		dot(colGreen, "ok", sum.Running),
		dot(colAmber, "pend", sum.Pending),
		dot(colRed, "crash", sum.CrashLoop),
		dot(colOrange, "back", sum.BackOff),
		dot(colRed, "fail", sum.Failed),
		dot(colDim, "unk", sum.Unknown),
	}
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if p != "" {
			out = append(out, p)
		}
	}
	return strings.Join(out, " ")
}

func (m Model) renderMid() string {
	if m.mode == viewText {
		return m.pane(m.textTitle, true, m.width, m.lay.midH+m.lay.eventsH, m.reader.View())
	}
	if m.mode == viewFlux {
		return m.renderFluxPane()
	}
	main := m.renderMainPane()
	if m.lay.detailsW <= 0 {
		return main
	}
	return lipgloss.JoinHorizontal(lipgloss.Top, main, m.renderDetailsPane())
}

func (m Model) renderMainPane() string {
	cw, ch := m.lay.mainContentSize()
	var title, body string
	if m.mode == viewTable {
		title = "pods · table"
		body = m.podTable.View()
	} else {
		title = "habitat"
		body = m.renderHabitat(cw, clampPos(ch-1))
	}
	return m.pane(title, m.focus == focusMain, m.lay.mainW, m.lay.midH, body)
}

func (m Model) renderDetailsPane() string {
	cw, _ := m.lay.detailsContentSize()
	return m.pane("details", m.focus == focusDetails,
		m.lay.detailsW, m.lay.midH, m.renderDetails(cw))
}

func (m Model) renderEvents() string {
	if m.mode == viewText || m.mode == viewFlux {
		return ""
	}
	title := fmt.Sprintf("events · %d", len(m.cluster.Events))
	return m.pane(title, m.focus == focusEvents, m.width, m.lay.eventsH, m.events.View())
}

func (m Model) renderFooter() string {
	if m.searching {
		return m.styles.footer.Render(truncate(m.search.View(), m.width-2))
	}
	if m.cmdMode {
		return m.styles.cmdPrompt.Render(truncate(m.cmd.View(), m.width-2))
	}
	var hints string
	switch m.mode {
	case viewFlux:
		hints = m.footerHints([][2]string{
			{"j/k", "move"}, {"r", "reconcile"}, {"s", "suspend"},
			{"enter", "message"}, {"esc", "back"}, {":", "cmd"}, {"q", "quit"},
		})
	case viewText:
		hints = m.footerHints([][2]string{
			{"j/k", "scroll"}, {"r", "refresh"}, {"esc", "back"},
		})
	default:
		hints = m.footerHints([][2]string{
			{":", "cmd"}, {"/", "filter"}, {"v", "view"}, {"f", "flux"},
			{"l", "logs"}, {"d", "describe"}, {"s", "shell"}, {"^d", "delete"},
			{"?", "help"},
		})
	}
	return m.styles.footer.Render(truncate(hints, m.width-2))
}

// footerHints renders "key desc" pairs with gold keys and faint descriptions.
func (m Model) footerHints(pairs [][2]string) string {
	parts := make([]string, 0, len(pairs))
	for _, p := range pairs {
		parts = append(parts, m.styles.footerKey.Render(p[0])+" "+p[1])
	}
	return strings.Join(parts, "  ")
}

// pane draws a titled, bordered box of outer size w×h around body.
func (m Model) pane(title string, focused bool, w, h int, body string) string {
	if w <= 0 || h <= 0 {
		return ""
	}
	st := m.styles.paneInactive
	if focused {
		st = m.styles.paneActive
	}
	innerW := clampPos(w - 2)
	innerH := clampPos(h - 2)
	titleLine := m.styles.paneTitle.Render(truncate(title, innerW))
	bodyBox := fitHeight(body, clampPos(innerH-1))
	inner := lipgloss.JoinVertical(lipgloss.Left, titleLine, bodyBox)
	return st.Width(innerW).Height(innerH).MaxWidth(w).MaxHeight(h).Render(inner)
}

// --- flux view ---------------------------------------------------------------

func (m Model) renderFluxPane() string {
	h := m.lay.midH + m.lay.eventsH
	w := m.width
	innerW := clampPos(w - 4)

	ready, failed, suspended := 0, 0, 0
	for _, fx := range m.cluster.Flux {
		switch {
		case fx.Suspended:
			suspended++
		case fx.Ready == "True":
			ready++
		case fx.Ready == "False":
			failed++
		}
	}
	title := fmt.Sprintf("flux · %d objects · %s · %s · %s",
		len(m.cluster.Flux),
		lipgloss.NewStyle().Foreground(colGreen).Render(fmt.Sprintf("%d ready", ready)),
		lipgloss.NewStyle().Foreground(colRed).Render(fmt.Sprintf("%d failing", failed)),
		lipgloss.NewStyle().Foreground(colGray).Render(fmt.Sprintf("%d suspended", suspended)))

	if len(m.cluster.Flux) == 0 {
		return m.pane(title, true, w, h, m.styles.emptyHint.Render("no flux objects found"))
	}

	// Column widths.
	kindW, nsW, readyW, revW, ageW := 14, 14, 7, 22, 6
	nameW := clampPos(innerW - kindW - nsW - readyW - revW - ageW - 10)

	head := lipgloss.NewStyle().Foreground(colMuted).Render(
		padRight("KIND", kindW) + "  " + padRight("NAME", nameW) + "  " +
			padRight("NAMESPACE", nsW) + "  " + padRight("READY", readyW) + "  " +
			padRight("REVISION", revW) + "  " + padRight("AGE", ageW))

	visible := clampPos(h - 4) // borders + title + header line
	start := 0
	if m.fluxSelected >= visible {
		start = m.fluxSelected - visible + 1
	}

	rows := []string{head}
	for i := start; i < len(m.cluster.Flux) && i-start < visible; i++ {
		fx := m.cluster.Flux[i]
		readyTxt := fx.Ready
		col := statusColor(fx.Health())
		if fx.Suspended {
			readyTxt = "⏸"
		}
		line := padRight(fx.Kind, kindW) + "  " +
			padRight(truncate(fx.Name, nameW), nameW) + "  " +
			padRight(truncate(fx.Namespace, nsW), nsW) + "  " +
			padRight(readyTxt, readyW) + "  " +
			padRight(truncate(fx.Revision, revW), revW) + "  " +
			padRight(fx.Age, ageW)
		if i == m.fluxSelected {
			rows = append(rows, lipgloss.NewStyle().
				Foreground(colBlack).Background(colGold).Bold(true).
				Render(truncate(line, innerW)))
		} else {
			rows = append(rows, lipgloss.NewStyle().Foreground(col).Render(truncate(line, innerW)))
		}
	}

	if fx, ok := m.selectedFlux(); ok {
		src := fx.Source
		if src == "" {
			src = "-"
		}
		detail := fmt.Sprintf("src %s — %s", src, fx.Message)
		rows = append(rows, m.styles.detailKey.Render(truncate(detail, innerW)))
	}

	return m.pane(title, true, w, h, strings.Join(rows, "\n"))
}

// --- confirm modal -----------------------------------------------------------

func (m Model) renderConfirm() string {
	msg := lipgloss.NewStyle().Foreground(colText).Render("release pod "+m.confirmKey+"?") +
		"\n\n" +
		m.styles.detailKey.Render("this deletes the pod — its owner may respawn it") +
		"\n\n" +
		m.styles.helpKey.Render("y") + m.styles.helpDesc.Render(" delete   ") +
		m.styles.helpKey.Render("any other key") + m.styles.helpDesc.Render(" cancel")
	box := m.styles.confirmBox.Render(msg)
	return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, box)
}

// --- habitat view ----------------------------------------------------------

type podGroup struct {
	name           string
	ready          bool
	cpu, mem       string
	cpuPct, memPct int
	pods           []state.PodView
}

func (m Model) podGroups() []podGroup {
	pods := m.visiblePods()
	byNode := map[string][]state.PodView{}
	known := map[string]bool{}
	for _, p := range pods {
		byNode[p.NodeName] = append(byNode[p.NodeName], p)
	}

	groups := make([]podGroup, 0, len(m.cluster.Nodes)+1)
	for _, n := range m.cluster.Nodes {
		known[n.Name] = true
		gp := byNode[n.Name]
		if len(gp) == 0 && (m.filter != "" || m.nsFilter != "") {
			continue // hide empty habitats while filtering
		}
		groups = append(groups, podGroup{n.Name, n.Ready, n.CPUText, n.MemoryText, n.CPUPercent(), n.MemPercent(), gp})
	}

	var orphans []state.PodView
	for _, p := range pods {
		if !known[p.NodeName] {
			orphans = append(orphans, p)
		}
	}
	if len(orphans) > 0 {
		groups = append(groups, podGroup{"(unscheduled)", false, "-", "-", -1, -1, orphans})
	}
	return groups
}

func (m Model) renderHabitat(w, h int) string {
	groups := m.podGroups()
	if len(groups) == 0 {
		return m.styles.emptyHint.Render("no pods match — press / to change the filter, or esc to clear")
	}

	habInnerW := clampPos(w - 4) // node box border (2) + padding (2)
	perRow := habInnerW / podCardOuterW
	if perRow < 1 {
		perRow = 1
	}

	sel, hasSel := m.selectedPod()
	blocks := make([]string, len(groups))
	selBlock := -1
	for gi, g := range groups {
		cards := make([]string, 0, len(g.pods))
		for _, p := range g.pods {
			active := hasSel && p.Key() == sel.Key()
			if active {
				selBlock = gi
			}
			cards = append(cards, m.renderPodCard(p, active))
		}
		blocks[gi] = m.renderNodeBox(g, cards, perRow, habInnerW)
	}

	full := lipgloss.JoinVertical(lipgloss.Left, blocks...)
	lines := strings.Split(full, "\n")
	if len(lines) <= h {
		return full
	}

	// Scroll so the selected critter's habitat stays on screen. Block heights
	// are constant between snapshots (frames are fixed-size), so this offset
	// is stable across animation ticks.
	offset := 0
	if selBlock >= 0 {
		start := 0
		for i := 0; i < selBlock; i++ {
			start += lipgloss.Height(blocks[i])
		}
		end := start + lipgloss.Height(blocks[selBlock])
		if end > h {
			offset = end - h
		}
		if offset > start {
			offset = start
		}
	}
	if max := len(lines) - h; offset > max {
		offset = max
	}
	if offset < 0 {
		offset = 0
	}
	return strings.Join(lines[offset:offset+h], "\n")
}

func (m Model) renderNodeBox(g podGroup, cards []string, perRow, innerW int) string {
	titleStyle := m.styles.nodeTitle
	statusGlyph := "●"
	if !g.ready {
		titleStyle = m.styles.nodeTitleDown
		statusGlyph = "○"
	}
	load := ""
	if g.cpuPct >= 0 {
		load = fmt.Sprintf(" · cpu %d%% · mem %d%%", g.cpuPct, g.memPct)
	}
	titleTxt := fmt.Sprintf("%s %s · %s · %s%s · %d pods",
		statusGlyph, g.name, g.cpu, g.mem, load, len(g.pods))
	title := titleStyle.Render(truncate(titleTxt, innerW))

	var body string
	if len(cards) == 0 {
		body = m.styles.emptyHint.Render("(no pods here yet)")
	} else {
		body = packCards(cards, perRow)
	}
	inner := lipgloss.JoinVertical(lipgloss.Left, title, body)
	return m.styles.nodeBox.Width(innerW).Render(inner)
}

func packCards(cards []string, perRow int) string {
	rows := make([]string, 0, (len(cards)+perRow-1)/perRow)
	for i := 0; i < len(cards); i += perRow {
		end := i + perRow
		if end > len(cards) {
			end = len(cards)
		}
		rows = append(rows, lipgloss.JoinHorizontal(lipgloss.Top, cards[i:end]...))
	}
	return lipgloss.JoinVertical(lipgloss.Left, rows...)
}

func (m Model) renderPodCard(p state.PodView, active bool) string {
	col := statusColor(p.Status)

	lines := []string{
		lipgloss.NewStyle().Bold(true).Foreground(colText).
			Render(truncate(p.Name, podCardInnerW)),
	}
	if m.cluster.AllNamespaces {
		lines = append(lines, m.styles.detailKey.Render(truncate(p.Namespace, podCardInnerW)))
	}

	frame := critters.Frame(p.Critter, p.CritterState, m.tick)
	// Pixel sprites carry truecolor ANSI per cell; lipgloss's width/align
	// pass mismeasures the byte length and wraps mid-escape into stripes.
	// Pad+center manually for pixel mode and let the colors speak for themselves.
	var frameBlock string
	if critters.PixelLoaded() {
		frameBlock = centerPixelFrame(frame, podCardInnerW)
	} else {
		frameBlock = lipgloss.NewStyle().Foreground(col).
			Width(podCardInnerW).Align(lipgloss.Center).Render(frame)
	}
	lines = append(lines, frameBlock)

	lines = append(lines,
		lipgloss.NewStyle().Bold(true).Foreground(col).
			Render(truncate(p.Status, podCardInnerW)))
	lines = append(lines,
		m.styles.detailKey.Render(truncate(
			fmt.Sprintf("↻%d  %s", p.Restarts, p.Age), podCardInnerW)))

	st := m.styles.podCard
	if active {
		st = m.styles.podCardActive
	}
	return st.Width(podCardInnerW).Render(lipgloss.JoinVertical(lipgloss.Left, lines...))
}

// --- details view ----------------------------------------------------------

// mood translates a pod status into kubagachi's tamagotchi vocabulary.
func mood(status string) string {
	switch status {
	case state.StatusRunning:
		return "content ♥"
	case state.StatusPending:
		return "curious"
	case state.StatusCompleted:
		return "resting"
	case state.StatusCrashLoop:
		return "sick — needs care"
	case state.StatusBackOff:
		return "sleepy"
	case state.StatusImagePull:
		return "hungry — waiting on image"
	case state.StatusOOMKilled:
		return "overfed (OOM)"
	case state.StatusFailed:
		return "hurt"
	case state.StatusTerminating:
		return "fading away"
	default:
		return "lost"
	}
}

func (m Model) renderDetails(w int) string {
	pod, ok := m.selectedPod()
	if !ok {
		return m.styles.emptyHint.Render("no pod selected\n\nuse the arrow keys to\npick a critter")
	}
	col := statusColor(pod.Status)

	var lines []string
	frame := critters.Frame(pod.Critter, pod.CritterState, m.tick)
	lines = append(lines,
		lipgloss.NewStyle().Foreground(col).Render(frame),
		"",
		lipgloss.NewStyle().Bold(true).Foreground(col).Render(truncate(strings.ToUpper(pod.Status), w)),
		"",
	)

	add := func(k, v string) {
		key := m.styles.detailKey.Render(padRight(k, 11))
		lines = append(lines, key+truncate(v, clampPos(w-11)))
	}
	add("mood", mood(pod.Status))
	add("pod", pod.Name)
	add("namespace", pod.Namespace)
	add("node", orDash(pod.NodeName))
	add("owner", orDash(pod.Owner))
	add("phase", orDash(pod.Phase))
	add("reason", orDash(pod.Reason))
	add("restarts", fmt.Sprintf("%d", pod.Restarts))
	add("ready", pod.ReadyText())
	add("age", pod.Age)
	add("critter", fmt.Sprintf("%s · %s", pod.Critter, pod.CritterState))

	lines = append(lines, "", m.styles.paneTitle.Render("containers"))
	if len(pod.Containers) == 0 {
		lines = append(lines, m.styles.emptyHint.Render("  (none reported)"))
	}
	for _, c := range pod.Containers {
		mark := lipgloss.NewStyle().Foreground(colRed).Render("●")
		if c.Ready {
			mark = lipgloss.NewStyle().Foreground(colGreen).Render("●")
		}
		lines = append(lines, mark+" "+truncate(c.Name, clampPos(w-2)))
		sub := fmt.Sprintf("    %s · ↻%d", c.State, c.RestartCount)
		if c.Reason != "" {
			sub += " · " + c.Reason
		}
		lines = append(lines, m.styles.detailKey.Render(truncate(sub, w)))
		if c.State == "terminated" {
			lines = append(lines,
				m.styles.detailKey.Render(truncate(fmt.Sprintf("    exit code: %d", c.ExitCode), w)))
		}
	}
	return strings.Join(lines, "\n")
}

// --- help screen -----------------------------------------------------------

func (m Model) renderHelp() string {
	var b strings.Builder
	b.WriteString(m.styles.headerTitle.Render("kubagachi — keybindings"))
	b.WriteString("\n\n")
	for _, kb := range m.keys.helpEntries() {
		h := kb.Help()
		b.WriteString(m.styles.helpKey.Render(padRight(h.Key, 12)))
		b.WriteString(m.styles.helpDesc.Render(h.Desc))
		b.WriteString("\n")
	}
	b.WriteString("\n")
	b.WriteString(m.styles.helpKey.Render(padRight(":", 12)))
	b.WriteString(m.styles.helpDesc.Render("commands: pods · habitat · flux · events · ns <name> · all · quit"))
	b.WriteString("\n\n")
	b.WriteString(m.styles.emptyHint.Render("press ? or esc to close this screen"))
	box := m.styles.paneActive.Padding(1, 3).Render(b.String())
	return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, box)
}

// --- table view + event feed -----------------------------------------------

func tableColumns(w int) []table.Column {
	if w < 50 {
		w = 50
	}
	const ns, status, ready, restarts, age, critter = 14, 12, 6, 8, 5, 9
	rest := w - (ns + status + ready + restarts + age + critter)
	if rest < 20 {
		rest = 20
	}
	podW := rest * 60 / 100
	nodeW := rest - podW
	return []table.Column{
		{Title: "NAMESPACE", Width: ns},
		{Title: "POD", Width: podW},
		{Title: "NODE", Width: nodeW},
		{Title: "STATUS", Width: status},
		{Title: "READY", Width: ready},
		{Title: "RESTARTS", Width: restarts},
		{Title: "AGE", Width: age},
		{Title: "CRITTER", Width: critter},
	}
}

func (m *Model) refreshTable() {
	pods := m.visiblePods()
	rows := make([]table.Row, 0, len(pods))
	for _, p := range pods {
		rows = append(rows, table.Row{
			p.Namespace, p.Name, orDash(p.NodeName), p.Status,
			p.ReadyText(), fmt.Sprintf("%d", p.Restarts), p.Age, p.Critter,
		})
	}
	m.podTable.SetRows(rows)
	if m.selected >= 0 && m.selected < len(rows) {
		m.podTable.SetCursor(m.selected)
	}
}

func (m *Model) refreshEvents() {
	ew, _ := m.lay.eventsContentSize()
	if ew <= 0 {
		ew = 80
	}
	if len(m.cluster.Events) == 0 {
		m.events.SetContent(m.styles.emptyHint.Render("no events reported"))
		return
	}
	var b strings.Builder
	for i, e := range m.cluster.Events {
		if i > 0 {
			b.WriteByte('\n')
		}
		b.WriteString(renderEventLine(e, ew))
	}
	m.events.SetContent(b.String())
}

func renderEventLine(e state.EventView, w int) string {
	typ := e.Type
	if typ == "" {
		typ = "Normal"
	}
	typeCol := colFaint
	if typ == "Warning" {
		typeCol = colOrange
	}
	age := fmt.Sprintf("%-5s", e.Time)
	tag := lipgloss.NewStyle().Foreground(typeCol).Render(padRight(typ, 8))
	rest := fmt.Sprintf("%s %s — %s", e.Reason, e.Object, e.Message)
	avail := clampPos(w - 5 - 1 - 8 - 1)
	return lipgloss.NewStyle().Foreground(colMuted).Render(age) + " " + tag + " " +
		lipgloss.NewStyle().Foreground(colText).Render(truncate(rest, avail))
}

// --- small helpers ---------------------------------------------------------

func truncate(s string, w int) string {
	if w <= 0 {
		return ""
	}
	r := []rune(s)
	if len(r) <= w {
		return s
	}
	if w == 1 {
		return "…"
	}
	return string(r[:w-1]) + "…"
}

func padRight(s string, w int) string {
	r := []rune(s)
	if len(r) >= w {
		return string(r[:w])
	}
	return s + strings.Repeat(" ", w-len(r))
}

func padLineTo(s string, w int) string {
	cw := lipgloss.Width(s)
	if cw < w {
		return s + strings.Repeat(" ", w-cw)
	}
	return s
}

func fitHeight(s string, h int) string {
	if h <= 0 {
		return ""
	}
	lines := strings.Split(s, "\n")
	for len(lines) < h {
		lines = append(lines, "")
	}
	return strings.Join(lines[:h], "\n")
}

func orDash(s string) string {
	if s == "" {
		return "-"
	}
	return s
}
