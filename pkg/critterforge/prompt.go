package critterforge

import "strings"

// promptVersion participates in every cache key. Bump it whenever the prompt
// templates below change in a way that would meaningfully shift sprite output.
const promptVersion = "v3"

// requirementsBlock is the hard-constraints list every generation prompt
// repeats verbatim. Kept identical between base and derived-state prompts
// so style stays consistent regardless of which path produced the sprite.
const requirementsBlock = `Requirements:
- exact sprite style, not illustration
- full body visible
- centered with 4 pixels of padding
- hard square pixels only
- no gradients
- no anti-aliasing
- limited palette: dark outline, body color, lighter highlights, darker shadow
- transparent background with real alpha channel
- no checkerboard background
- no text
- no UI frame
- no blur
- no soft shading
- readable at 32x32
- export as PNG sprite`

// basePrompt builds the prompt for the canonical "running" sprite. Only
// called when the caller did not supply a per-critter ReferencePNG. If
// hasStyleRef is true the caller will attach a global style-reference
// image, and the prompt is adjusted to acknowledge it.
func basePrompt(s Spec, hasStyleRef bool) string {
	var b strings.Builder
	if hasStyleRef {
		b.WriteString("The attached image is a STYLE REFERENCE — match its exact pixel-art aesthetic: same palette discipline, line weight, proportions, shading approach, and rendering conventions.\n\n")
	}
	b.WriteString("Create a TRUE 32x32 pixel art sprite of ")
	if s.Description != "" {
		b.WriteString(s.Description)
	} else {
		b.WriteString(`a cute mascot for the service "`)
		b.WriteString(s.Name)
		b.WriteString(`"`)
	}
	b.WriteString(" for a Kubernetes terminal dashboard.\n\n")
	b.WriteString(requirementsBlock)
	b.WriteString("\n\nPose:\n")
	b.WriteString("- alert and healthy\n")
	b.WriteString("- neutral standing pose, front-facing\n")
	b.WriteString("- eyes open, calm friendly expression\n")
	b.WriteString("- all defining features clearly visible\n")
	b.WriteString("- canonical 'running' state — will be reused as the visual reference for every other variant")
	if s.Instructions != "" {
		b.WriteString("\n\nAdditional instructions:\n- ")
		b.WriteString(s.Instructions)
	}
	return b.String()
}

// referenceBasePrompt converts a user-supplied reference image into a pixel-art
// running sprite. The reference fixes the character's identity; the prompt
// forces a full conversion to the sprite style, because references are often
// smooth vector/cartoon art that must be re-drawn as hard pixels (not copied).
func referenceBasePrompt(s Spec) string {
	var b strings.Builder
	b.WriteString("The attached image is the CHARACTER REFERENCE — it defines this character's identity: species, colors, costume, hat, props, and distinguishing features. Recreate THIS EXACT character as a TRUE 32x32 pixel art sprite.\n\n")
	b.WriteString("CRITICAL: do NOT copy the reference's smooth, vector, or illustrated rendering. Re-draw the whole character from scratch as hard-edged pixel art — every shape made of square pixels. Keep it instantly recognizable (same costume, same props, same palette) but fully converted to crisp sprite style.\n\n")
	b.WriteString(requirementsBlock)
	b.WriteString("\n\nPose:\n")
	b.WriteString("- alert and healthy\n")
	b.WriteString("- neutral standing pose, front-facing\n")
	b.WriteString("- eyes open, calm friendly expression\n")
	b.WriteString("- all defining features clearly visible\n")
	b.WriteString("- canonical 'running' state — will be reused as the visual reference for every other variant")
	if s.Instructions != "" {
		b.WriteString("\n\nAdditional instructions:\n- ")
		b.WriteString(s.Instructions)
	}
	return b.String()
}

// statePrompt builds the prompt for a non-base state. The caller must pass
// the canonical base PNG as a reference image alongside this prompt. If
// hasStyleRef is true the caller is also attaching a global style-reference
// image first; the prompt is adjusted accordingly.
func statePrompt(s Spec, state State, hasStyleRef bool) string {
	var b strings.Builder
	if hasStyleRef {
		b.WriteString("Two images are attached:\n")
		b.WriteString("  1. FIRST = STYLE REFERENCE — match its exact pixel-art aesthetic (palette discipline, line weight, proportions, shading).\n")
		b.WriteString("  2. SECOND = CHARACTER REFERENCE — the canonical sprite for THIS character. Keep its species, proportions, palette, and distinguishing features identical.\n\n")
	} else {
		b.WriteString("The attached image is the CHARACTER REFERENCE — the canonical sprite for this character. Keep its species, proportions, palette, and distinguishing features identical.\n\n")
	}
	b.WriteString("Create a TRUE 32x32 pixel art sprite of the same character ")
	if s.Description != "" {
		b.WriteString("(")
		b.WriteString(s.Description)
		b.WriteString(") ")
	}
	b.WriteString("for a Kubernetes terminal dashboard, transformed into the state described below.\n\n")
	b.WriteString(requirementsBlock)
	b.WriteString("\n\nEmotion must be EXAGGERATED to cartoon-extreme levels — push the face, body language, and overlays so the state reads instantly at 32x32. Never subtle.\n\nPose:\n")
	b.WriteString(stateRecipes[state])
	if s.Instructions != "" {
		b.WriteString("\n\nAdditional instructions:\n- ")
		b.WriteString(s.Instructions)
	}
	return b.String()
}

// stateRecipes is the per-state mood/treatment recipe, formatted as bullet
// lines for the prompt's Pose section. Each entry should describe a visual
// transformation of the base sprite, not a redesign. Crank everything: the
// sprites only have ~32x32 pixels to telegraph mood, so subtle reads as
// invisible.
var stateRecipes = map[State]string{
	StatePending: `- waiting IMPATIENTLY, on the edge of action
- normal character coloring preserved
- HUGE round wide-open eyes, intense focused stare
- single prominent sweat drop on the forehead
- a bright yellow clock or hourglass icon floating beside the head, exaggerated and obvious
- body leaned forward, fidgety, about-to-pounce posture`,

	StateCompleted: `- TRIUMPHANT, just won the game
- strong cheerful green color wash applied over the entire sprite (tint, do not replace base colors)
- HUGE beaming smile, eyes squinted into happy arcs (^_^) or glittering stars
- chest puffed out, arms or paws thrown up in a victory pose
- bright sparkle marks, shine glints, or tiny stars popping around the head
- a small glowing halo or burst of light behind the character`,

	StateCrashLoop: `- ABSOLUTE PANIC, end-of-the-world distress
- INTENSE red color wash applied over the entire sprite, with small flame licks creeping up around the feet or sides
- HUGE X_X eyes (big dramatic crosses), mouth wide open in a horrified scream
- multiple steam puffs, big sweat drops, lightning bolts, exclamation marks ALL around the head
- visible shake/vibration lines around the body
- completely disheveled, off-balance, mid-fall pose — total chaos`,

	StateBackOff: `- KNOCKED OUT, completely passed out
- heavy brownish, muted, drained color wash applied over the entire sprite
- eyes squeezed shut, droopy heavy eyelids
- LARGE bold "Zzz" letters floating above the head, exaggerated size
- a small drool drip or snore bubble at the mouth
- body fully slumped, sagging, sliding over — not just sitting, fully collapsed asleep`,

	StateTerminating: `- DYING gracefully, dissolving into the void
- desaturated washed-out gray color wash applied over the entire sprite
- AT LEAST 30% of the body visibly breaking apart into drifting pixel particles, fading edges
- eyes closed peacefully with a single tear, or a tiny soul/spirit wisp lifting off the head
- tragic but serene expression, head tilted, drooping posture
- additional ghostly upward-drifting particles or a faint glow to emphasize the departure`,

	StateUnknown: `- state is completely UNKNOWABLE, the character is a mystery
- render ONLY the outline silhouette of the character as solid clean white lines on a fully transparent background
- no fill, no color, NO facial features inside the outline
- lines should be clean and continuous, not dashed or dotted
- like a blueprint trace or ghost-outline of the character, totally hollow inside`,
}
