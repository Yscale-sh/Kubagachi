package critters

import "strings"

// FrameHeight is the fixed number of text lines every critter frame occupies.
// Keeping it constant stops pod cards from jumping around as critters animate.
const FrameHeight = 4

// critterSpec describes the stable "alive" look of one animal. State-specific
// frames are derived from this spec by the frame builder below.
type critterSpec struct {
	name    string
	ears    string
	feet    string
	feetAlt string
	lp, rp  string
}

// specs is the registry of available critters. Order is stable so the
// deterministic hash assignment never shifts between releases.
var specs = []critterSpec{
	{"cat", ` /\_/\ `, ` > ^ < `, `  >^<  `, "(", ")"},
	{"dog", ` /^-^\ `, ` ^U_U^ `, ` ^U U^ `, "(", ")"},
	{"rabbit", ` (\_/) `, ` ("')  `, `  ("') `, "(", ")"},
	{"turtle", ` .---. `, ` ~()() `, ` ()()~ `, "(", ")"},
	{"raccoon", ` /^v^\ `, ` =m_m= `, ` =m m= `, "(", ")"},
	{"fox", ` /v_v\ `, ` ~'='~ `, ` ~='=~ `, "(", ")"},
	{"ghost", `  .-.  `, ` ~~~~~ `, ` ~ ~ ~ `, "(", ")"},
	{"blob", `  ___  `, ` \___/ `, ` \_ _/ `, "{", "}"},
}

var specByName = func() map[string]critterSpec {
	m := make(map[string]critterSpec, len(specs))
	for _, s := range specs {
		m[s.name] = s
	}
	return m
}()

// pad forces a frame to exactly FrameHeight lines.
func pad(lines []string) []string {
	for len(lines) < FrameHeight {
		lines = append(lines, "")
	}
	return lines[:FrameHeight]
}

// face renders the critter's animated face line "( eyes )".
func face(s critterSpec, eyes string) string {
	return s.lp + " " + eyes + " " + s.rp
}

// buildFrame returns the FrameHeight lines for a critter in a given state at
// the given animation tick. Animation is intentionally subtle: at most one
// element moves per tick so the habitat view never looks noisy.
func buildFrame(s critterSpec, status string, tick int) []string {
	switch status {
	case "running":
		eyes := "o.o"
		if tick%6 == 3 {
			eyes = "-.-" // occasional blink
		}
		feet := s.feet
		if tick%2 == 1 {
			feet = s.feetAlt
		}
		return pad([]string{s.ears, face(s, eyes), feet})

	case "pending":
		eyes := "?.?"
		if tick%2 == 1 {
			eyes = "?.o"
		}
		return pad([]string{s.ears, face(s, eyes), s.feet})

	case "crashloop":
		if tick%2 == 0 {
			return pad([]string{s.ears, face(s, "x.x"), ` /|||\ `})
		}
		return pad([]string{` _____ `, face(s, "X.X"), ` /|||\ `, `  RIP  `})

	case "backoff":
		z := []string{`  z    `, `   z   `, `  z Z  `}[tick%3]
		return pad([]string{z, s.ears, face(s, "-.-"), s.feet})

	case "imagepull":
		box := `[box?]`
		if tick%2 == 1 {
			box = `[box.]`
		}
		return pad([]string{s.ears, face(s, ">.<"), " " + box + " "})

	case "terminating":
		switch tick % 3 {
		case 0:
			return pad([]string{`  .-.  `, ` ( -- )`, `  )-(  `, `  ' '  `})
		case 1:
			return pad([]string{`  . .  `, ` ( -- )`, `  ) (  `})
		default:
			return pad([]string{`   .   `, `  ' '  `, `   .   `})
		}

	case "unknown":
		switch tick % 3 {
		case 0:
			return pad([]string{` ????? `, face(s, "?.?"), ` ????? `})
		case 1:
			return pad([]string{` ##### `, face(s, "#.#"), ` ##### `})
		default:
			return pad([]string{` ~~~~~ `, face(s, "..."), ` ~~~~~ `})
		}

	case "completed":
		eyes := "-.-"
		if tick%4 == 0 {
			eyes = "^.^"
		}
		return pad([]string{s.ears, face(s, eyes), s.feet, ` done  `})

	case "oomkilled":
		return pad([]string{` _____ `, face(s, "x_x"), `  OOM  `})

	case "failed":
		eyes := ";_;"
		if tick%3 == 0 {
			eyes = ".__."
		}
		return pad([]string{s.ears, face(s, eyes), s.feet, ` fail  `})

	default:
		return pad([]string{s.ears, face(s, "o.o"), s.feet})
	}
}

// Frame returns the multi-line frame for the named critter in the given
// status. If a pixel sprite set has been loaded (LoadPixelSprites) and
// covers this (critter, status), an inline-image placeholder is returned and
// the tick is ignored (current sprites are single-frame). Otherwise the
// built-in animated ASCII frame is returned; unknown critter names fall back
// to the first registered critter.
func Frame(critter, status string, tick int) string {
	if frame := pixelFrame(critter, status); frame != "" {
		return frame
	}
	s, ok := specByName[critter]
	if !ok {
		s = specs[0]
	}
	return strings.Join(buildFrame(s, status, tick), "\n")
}
