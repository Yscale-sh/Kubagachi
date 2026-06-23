package critterforge

import (
	"fmt"
	"strings"
)

// StatusSpec is the authoritative description of one pod state. It is used
// by both the keyed-sheet generator (one frame per status) and the
// per-state animation generator (8 frames per status). Keeping both
// pipelines reading the same spec is what prevents "Running" looking
// drastically different across the two surfaces.
type StatusSpec struct {
	// Slug is the canonical state name used in filenames and code
	// (lowercase, no spaces), e.g. "running", "crashloop", "error".
	Slug string
	// Display is the human/Kubernetes name shown in the prompt to the model
	// so it locks onto the right semantic concept ("CrashLoopBackOff" not
	// "crashloop").
	Display string

	VisualGoal    string
	Pose          string
	Face          string
	Effects       string
	ColorBehavior string
	Avoid         string
	PromptText    string
}

// StatusOrder is the canonical left-to-right column order for keyed sheets,
// and the alphabet for all per-state generators.
var StatusOrder = []string{
	"running",
	"pending",
	"completed",
	"crashloop",
	"backoff",
	"terminating",
	"unknown",
	"error",
}

// Statuses is the single source of truth. Edit only here.
var Statuses = map[string]StatusSpec{
	"running": {
		Slug:          "running",
		Display:       "Running",
		VisualGoal:    "healthy, alive, stable, production-normal",
		Pose:          "upright or confidently seated; balanced posture; full body visible; no distortion",
		Face:          "open normal eyes; calm or slightly happy expression; relaxed mouth",
		Effects:       "optional tiny green/blue healthy sparkle or subtle idle motion pixels; no warning marks",
		ColorBehavior: "normal mascot colors; clean dark outline; brightest and most complete version",
		Avoid:         "sleeping, damage, red pixels, ghosting, panic, fading",
		PromptText: "Running state: mascot is healthy and stable, upright or calmly seated, " +
			"normal open eyes, relaxed happy expression, full-color body, clean outline, " +
			"no damage, no glitching, no warning marks.",
	},
	"pending": {
		Slug:          "pending",
		Display:       "Pending",
		VisualGoal:    "waiting to start, not broken, slightly uncertain",
		Pose:          "still standing or sitting; slightly paused posture; one paw/limb lifted or tucked",
		Face:          "neutral or mildly worried; eyes open but less confident than Running",
		Effects:       "small pause/waiting pixels, tiny dot indicator, optional subtle question-like body language but no text",
		ColorBehavior: "normal colors but slightly muted compared to Running",
		Avoid:         "X eyes, sleeping, red error glow, full collapse",
		PromptText: "Pending state: mascot is waiting or idle, slightly uncertain but not failed, " +
			"neutral face, small paused posture, normal colors slightly muted, no damage, " +
			"no red warning pixels, no sleeping.",
	},
	"completed": {
		Slug:          "completed",
		Display:       "Completed",
		VisualGoal:    "successful, done, satisfied",
		Pose:          "relaxed, seated, lying peacefully, or small celebratory posture",
		Face:          "closed happy eyes or satisfied smile; peaceful expression",
		Effects:       "optional tiny gold sparkle/check-like success pixels, but no text or symbols if possible",
		ColorBehavior: "normal or gently warmer colors; clean outline",
		Avoid:         "distress, red pixels, ghost fading, unknown silhouette",
		PromptText: "Completed state: mascot looks finished and satisfied, relaxed posture, " +
			"happy closed eyes or peaceful smile, calm success energy, full-color body, " +
			"clean outline, optional tiny celebratory sparkle pixels, no warning effects.",
	},
	"crashloop": {
		Slug:          "crashloop",
		Display:       "CrashLoopBackOff",
		VisualGoal:    "hard failure and repeated crashing",
		Pose:          "distressed, unstable, slumped but still visible; body may shake/glitch",
		Face:          "X eyes, dizzy eyes, panicked mouth, or tongue-out failure expression",
		Effects:       "red glitch pixels around body, small red warning sparks above head, broken/flickering outline",
		ColorBehavior: "normal mascot colors tinted with red danger accents",
		Avoid:         "too cute/healthy; avoid making it just sleepy; must read as actively broken",
		PromptText: "CrashLoopBackOff state: mascot is visibly failed and unstable, X eyes or dizzy eyes, " +
			"distressed mouth, slumped or shaking pose, red glitch pixels and warning sparks " +
			"around the body, broken/flickering outline, still recognizable as the same mascot.",
	},
	"backoff": {
		Slug:          "backoff",
		Display:       "BackOff",
		VisualGoal:    "retry delay, exhausted but not fully dead",
		Pose:          "lying down, slumped, tired, or head resting on paws/limbs",
		Face:          "sleepy eyes, exhausted expression, annoyed or defeated but not X-eyed",
		Effects:       "small Z pixels allowed; minimal amber/orange retry-delay accents",
		ColorBehavior: "normal colors slightly dimmed; no heavy red failure glow",
		Avoid:         "X eyes, full ghost fade, unknown silhouette, heavy red crash effects",
		PromptText: "BackOff state: mascot is exhausted and waiting before retrying, lying down or slumped, " +
			"sleepy half-closed eyes, tired expression, optional small Z pixels, muted colors, " +
			"no X eyes, no full crash explosion.",
	},
	"terminating": {
		Slug:          "terminating",
		Display:       "Terminating",
		VisualGoal:    "being removed gracefully, disappearing",
		Pose:          "calm fading posture; body partly dissolving; can look sleepy or resigned",
		Face:          "closed eyes, neutral tired face, peaceful disappearing expression",
		Effects:       "right side or edges dissolving into square pixels; ghostlike transparency; fading tail/body",
		ColorBehavior: "pale/desaturated version of mascot; lower contrast; partially transparent look",
		Avoid:         "red error glow, panic, X eyes unless explicitly forced",
		PromptText: "Terminating state: mascot is gracefully disappearing, pale and ghostlike, " +
			"body edges dissolving into small square pixels, calm closed eyes or neutral face, " +
			"desaturated colors, partially faded silhouette, no red warning glow.",
	},
	"unknown": {
		Slug:          "unknown",
		Display:       "Unknown",
		VisualGoal:    "unreachable, unclear, no reliable state",
		Pose:          "same mascot silhouette but obscured, shadowy, or incomplete",
		Face:          "only faint eyes visible, or no readable face",
		Effects:       "dotted/dashed outline, missing pixels, dark silhouette, question-mark-like uncertainty only through posture/effects",
		ColorBehavior: "dark navy/gray silhouette; minimal highlights; very low detail",
		Avoid:         "normal full-color mascot, red error state, happy expression",
		PromptText: "Unknown state: mascot appears as a dark obscured silhouette, dotted or dashed outline, " +
			"incomplete body made of missing pixels, faint eyes only, low-detail shadow form, " +
			"unclear and unreachable but still recognizable by silhouette.",
	},
	"error": {
		Slug:          "error",
		Display:       "Error",
		VisualGoal:    "explicit alert/failure, but not necessarily crash-looping",
		Pose:          "angry, alarmed, braced, or tense",
		Face:          "furrowed eyes, angry eyes, alarmed mouth, or shocked expression",
		Effects:       "red tint, red warning pixels, small alert sparks around body",
		ColorBehavior: "mascot colors strongly tinted red/pink; high danger contrast",
		Avoid:         "sleeping, ghost fade, calm success pose",
		PromptText: "Error state: mascot is in an alert failure condition, red-tinted body, " +
			"angry or alarmed face, tense pose, red warning pixels around the body, " +
			"strong danger mood, readable as an error but still the same mascot.",
	},
}

// StatusBlock renders a per-status section formatted for embedding inside a
// larger generation prompt.
func StatusBlock(slug string) string {
	s, ok := Statuses[slug]
	if !ok {
		return ""
	}
	var b strings.Builder
	fmt.Fprintf(&b, "%s (%s)\n", s.Display, s.VisualGoal)
	fmt.Fprintf(&b, "  Pose: %s\n", s.Pose)
	fmt.Fprintf(&b, "  Face: %s\n", s.Face)
	fmt.Fprintf(&b, "  Effects: %s\n", s.Effects)
	fmt.Fprintf(&b, "  Color: %s\n", s.ColorBehavior)
	fmt.Fprintf(&b, "  Avoid: %s\n", s.Avoid)
	return b.String()
}

// AllStatusBlocks renders every status section in canonical order, joined
// with a blank line between. Used by the keyed-sheet prompt.
func AllStatusBlocks() string {
	var b strings.Builder
	for i, slug := range StatusOrder {
		fmt.Fprintf(&b, "%d. %s", i+1, StatusBlock(slug))
		if i < len(StatusOrder)-1 {
			b.WriteString("\n")
		}
	}
	return b.String()
}
