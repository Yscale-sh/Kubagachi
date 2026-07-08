package tui

// layout holds the computed outer dimensions of every TUI region for a given
// terminal size. Outer dimensions include borders; content sizes are derived
// by subtracting 2 where a region is drawn inside a border.
type layout struct {
	width, height int

	headerH int
	footerH int
	statusH int
	eventsH int // outer height of the bottom event feed
	midH    int // outer height of the main+details row

	mainW    int // outer width of the habitat/table pane
	detailsW int // outer width of the details pane (0 when hidden)
}

const minPaneWidth = 30

// computeLayout splits a terminal of size w×h into region rectangles.
func computeLayout(w, h int, showStatus bool) layout {
	l := layout{width: w, height: h, headerH: 1, footerH: 1}
	if showStatus {
		l.statusH = 1
	}

	avail := h - l.headerH - l.footerH - l.statusH
	if avail < 8 {
		avail = 8
	}

	l.eventsH = 9
	if avail < 22 {
		l.eventsH = 6
	}
	if avail < 14 {
		l.eventsH = 4
	}
	l.midH = avail - l.eventsH
	if l.midH < 6 {
		l.midH = 6
		l.eventsH = avail - l.midH
		if l.eventsH < 3 {
			l.eventsH = 3
		}
	}

	l.detailsW = w * 32 / 100
	if l.detailsW < 30 {
		l.detailsW = 30
	}
	if l.detailsW > 48 {
		l.detailsW = 48
	}
	l.mainW = w - l.detailsW
	if l.mainW < minPaneWidth {
		// Terminal too narrow to show both panes side by side.
		l.mainW = w
		l.detailsW = 0
	}
	return l
}

// mainContentSize returns the usable inner width/height of the main pane.
func (l layout) mainContentSize() (int, int) {
	return clampPos(l.mainW - 2), clampPos(l.midH - 2)
}

// detailsContentSize returns the usable inner width/height of the details pane.
func (l layout) detailsContentSize() (int, int) {
	return clampPos(l.detailsW - 2), clampPos(l.midH - 2)
}

// eventsContentSize returns the usable inner width/height of the event feed.
func (l layout) eventsContentSize() (int, int) {
	return clampPos(l.width - 2), clampPos(l.eventsH - 2)
}

func clampPos(v int) int {
	if v < 0 {
		return 0
	}
	return v
}
