// Command spriteanim asks the image model to turn each source state sprite
// into a flip-book animation deck.
//
// Source contract:
//   - input:  critters/<name>/sprite-sheet-keyed.png, falling back to sprite-sheet.png
//   - output: critters/<name>/sprite-sheet-<state>.png
//
// The model sees the full keyed status sheet plus one cropped source state
// image, then generates one 8-frame horizontal animation sheet for that state.
package main

import (
	"bytes"
	"context"
	"errors"
	"flag"
	"fmt"
	"image"
	"image/draw"
	_ "image/jpeg"
	"image/png"
	"os"
	"os/signal"
	"path/filepath"
	"sort"
	"strings"
	"syscall"

	"github.com/joho/godotenv"

	"github.com/jakenesler/kubagachi/pkg/critterforge"
)

var states = []string{
	"running",
	"pending",
	"completed",
	"crashloop",
	"backoff",
	"terminating",
	"unknown",
	"failed",
}

var stateAnimationBrief = map[string]string{
	"running":     "a lively idle/run loop with body bounce, tiny foot/arm movement, and breathing motion",
	"pending":     "a waiting/loading loop with anticipation, scanning, small question/processing energy",
	"completed":   "a success loop with a satisfied pose, tiny celebration, sparkle/checkmark energy",
	"crashloop":   "an error loop with jitter, startled recoil, impact shake, red-alert energy",
	"backoff":     "a sleeping/backoff loop with drowsy breathing, drooping posture, subtle zzz energy",
	"terminating": "a shutdown loop where the character calmly fades or dissolves into pixel particles",
	"unknown":     "a confused/glitch loop with unstable pose, question energy, and readable uncertainty",
	"failed":      "a defeated failure loop with collapse/slump/error energy while remaining cute and readable",
}

// stateFrameRecipes describes EACH of the 8 frames per state, in order.
// Each entry is a list of 8 short phrases. gpt-image-1.5 at low/medium
// quality treats "8 frames of a loop" as "8 near-identical drawings" unless
// you describe every single frame, so we do.
var stateFrameRecipes = map[string][]string{
	"running": {
		"feet planted, body neutral, looking ahead alert",
		"slight knee bend, body lowered ~10% (compression)",
		"deepest crouch / lowest body position, arms slightly back",
		"pushing up, body rising, arms swinging forward",
		"peak of bounce, body fully extended, arms raised, slight smile",
		"starting to descend, body relaxing, arms coming down",
		"mid-descent, knees softening, arms returning to neutral",
		"settling back to neutral pose, ready to loop",
	},
	"pending": {
		"head tilted slightly left, eyes scanning, looking thoughtful",
		"head returning to center, eyes wider, anticipating",
		"head tilted slightly right, eyes scanning the other way",
		"head centered, small fidget — paw/hand twitch",
		"single sweat drop appears beside head, eyes a bit anxious",
		"sweat drop slides down, paw lifts briefly",
		"paw lowers, sweat drop fades",
		"back to neutral expectant pose, mid-blink",
	},
	"completed": {
		"normal pose with a small grin",
		"arms starting to raise, grin widening",
		"arms fully raised in V-victory, big smile, eyes ^_^",
		"small sparkle appears top-right, body bounces up slightly",
		"second sparkle top-left, body at peak bounce",
		"sparkles fade, body settling, arms still raised",
		"arms lowering, big smile remains",
		"arms down at sides, satisfied grin, ready to loop",
	},
	"crashloop": {
		"body upright, eyes wide-open in shock (O_O), tiny exclamation mark above head",
		"violent shake left, X eyes appearing, mouth open",
		"violent shake right, X eyes wider, sparks/jitter pixels around body",
		"body recoiling backward, X eyes, red flash overlay",
		"body falling forward, X eyes, dust/impact pixels at feet",
		"body crumpled, X eyes pinched, small flames flickering at edges",
		"slight recovery wobble, X eyes still, smoke wisp rising",
		"body upright again with shaken expression, ready to crash again",
	},
	"backoff": {
		"sitting down, eyes drooping, head nodding forward",
		"head nodded all the way, eyes closed, tiny Z floating up",
		"body slumping further, two Zs floating",
		"body fully lying down/curled, three Zs",
		"deep sleep pose, Zs at peak height",
		"single drool pixel at mouth, Zs starting to fade",
		"slight breathing twitch, one Z remaining",
		"back to drowsy sitting nod, restarting the doze",
	},
	"terminating": {
		"normal body but slightly desaturated, calm closed eyes",
		"~10% of body bottom edge breaking into drifting pixels",
		"~25% of body dissolving into pixels rising upward",
		"~40% dissolved, head still intact with peaceful expression",
		"~55% dissolved, only head and shoulders visible",
		"~70% dissolved, head fading, tiny soul-wisp rising",
		"~85% dissolved, just an outline and a few drifting pixels",
		"almost fully gone, faint silhouette with one drifting pixel",
	},
	"unknown": {
		"solid dark silhouette of the character, eyes glowing faintly",
		"silhouette with one dotted-line edge appearing",
		"silhouette with two dotted-line edges, eyes flicker brighter",
		"silhouette with question-mark pixel above head",
		"silhouette with question-mark in different position, eyes shifted",
		"silhouette mid-glitch, slight pixel displacement on one side",
		"silhouette resolving, dotted edges fading",
		"back to solid dark silhouette with glowing eyes",
	},
	"failed": {
		"body upright but red-tinted, frowning, small angry eyebrow tilt",
		"slight slump forward, red intensifies, tiny tear pixel",
		"body slumped more, eyes closed in defeat",
		"fully slumped, single drop falling from face",
		"body collapsed on ground, head down",
		"small angry red flash overlay around body",
		"body starting to rise again, defeated expression",
		"back to upright red-tinted frowning pose, looping",
	},
}

var stateDisplayName = map[string]string{
	"running":     "Running",
	"pending":     "Pending",
	"completed":   "Completed",
	"crashloop":   "CrashLoopBackOff",
	"backoff":     "BackOff",
	"terminating": "Terminating",
	"unknown":     "Unknown",
	"failed":      "Error",
}

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "spriteanim:", err)
		os.Exit(1)
	}
}

func run() error {
	_ = godotenv.Load()

	dir := flag.String("dir", "critters", "critters directory")
	input := flag.String("in", "critters.yaml", "input manifest (for workload animations)")
	provider := flag.String("provider", "gemini", "image provider: gemini | openai | flux")
	modelID := flag.String("model", "", "image model id (default: provider's default)")
	size := flag.String("size", "1536x1024", "image size (openai WxH) / aspect (gemini)")
	quality := flag.String("quality", "high", "image quality: low | medium | high")
	only := flag.String("only", "", "comma-separated critter names to generate (default: all)")
	stateOnly := flag.String("state", "", "single state/animation to generate (default: all)")
	force := flag.Bool("force", false, "overwrite existing state animation sheets")
	flag.Parse()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	model, err := critterforge.BuildImageModel(*provider, *modelID, *size, *quality)
	if err != nil {
		return err
	}

	// Workload animations come from the manifest; missing/unreadable is fine
	// (we just generate the eight base states).
	animsByCritter := map[string][]critterforge.InputAnimation{}
	if manifest, err := critterforge.LoadInputManifest(*input); err == nil {
		animsByCritter = critterforge.AnimationsByCritter(manifest)
	} else {
		fmt.Fprintf(os.Stderr, "spriteanim: no workload animations (%v)\n", err)
	}

	paths, err := sourceSheets(*dir)
	if err != nil {
		return err
	}
	sort.Strings(paths)
	if len(paths) == 0 {
		return errors.New("no sprite-sheet-keyed.png or sprite-sheet.png files found")
	}

	wantedCritters := csvSet(*only)
	for _, path := range paths {
		name := filepath.Base(filepath.Dir(path))
		if len(wantedCritters) > 0 && !wantedCritters[name] {
			continue
		}
		if err := generateCritter(ctx, model, path, name, *stateOnly, animsByCritter[name], *force); err != nil {
			return fmt.Errorf("%s: %w", name, err)
		}
	}
	return nil
}

// generateCritter renders the eight base health-state decks plus any workload
// animation decks declared for this critter. onlyState (when non-empty) limits
// generation to a single base state or animation by name.
func generateCritter(ctx context.Context, model critterforge.ImageModel, sheetPath, name, onlyState string, anims []critterforge.InputAnimation, force bool) error {
	fullSheet, frames, err := loadBaseFrames(sheetPath)
	if err != nil {
		return err
	}
	dir := filepath.Dir(sheetPath)

	writeDeck := func(state string, expected int, raw []byte) error {
		// Same keyer as the keyed status sheet: flood-fill the baked
		// checkerboard to real alpha and reflow into one clean row. Only the
		// normalized deck is written — no raw intermediate.
		data, err := critterforge.NormalizeKeyedSheet(raw, expected)
		if err != nil {
			return fmt.Errorf("%s normalize: %w", state, err)
		}
		return os.WriteFile(filepath.Join(dir, fmt.Sprintf("sprite-sheet-%s.png", state)), data, 0o644)
	}

	// Base health states.
	for _, state := range states {
		if onlyState != "" && onlyState != state {
			continue
		}
		base, ok := frames[state]
		if !ok {
			return fmt.Errorf("missing state %q in base sheet", state)
		}
		outPath := filepath.Join(dir, fmt.Sprintf("sprite-sheet-%s.png", state))
		if !force {
			if _, err := os.Stat(outPath); err == nil {
				fmt.Println("cached", outPath)
				continue
			}
		}
		fmt.Println("generating", outPath)
		raw, err := generateDeck(ctx, model, name, state, fullSheet, base)
		if err != nil {
			return fmt.Errorf("%s: %w", state, err)
		}
		if err := writeDeck(state, len(states), raw); err != nil {
			return err
		}
	}

	// Workload animation decks.
	for _, anim := range anims {
		if onlyState != "" && onlyState != anim.State {
			continue
		}
		baseName := anim.Base
		if baseName == "" {
			baseName = "running"
		}
		base, ok := frames[baseName]
		if !ok {
			return fmt.Errorf("animation %q: base state %q not in sheet", anim.State, baseName)
		}
		outPath := filepath.Join(dir, fmt.Sprintf("sprite-sheet-%s.png", anim.State))
		if !force {
			if _, err := os.Stat(outPath); err == nil {
				fmt.Println("cached", outPath)
				continue
			}
		}
		fmt.Println("generating", outPath, "(workload)")
		expected := len(anim.Frames)
		if expected == 0 {
			expected = 8
		}
		raw, err := model.GenerateSprite(ctx, customDeckPrompt(name, baseName, anim), fullSheet, base)
		if err != nil {
			return fmt.Errorf("%s: %w", anim.State, err)
		}
		if err := writeDeck(anim.State, expected, raw); err != nil {
			return err
		}
	}
	return nil
}

// customDeckPrompt builds the generation prompt for a workload animation. Unlike
// the base-state prompt it has no k8s StatusBlock — identity comes from the
// keyed sheet, the action comes from the theme + per-frame recipe.
func customDeckPrompt(critterName, baseName string, anim critterforge.InputAnimation) string {
	n := len(anim.Frames)
	if n == 0 {
		n = 8
	}
	var recipe strings.Builder
	for i, f := range anim.Frames {
		fmt.Fprintf(&recipe, "  Frame %d: %s\n", i+1, f)
	}
	return fmt.Sprintf(`Two reference images are attached:
1. FULL KEYED STATUS SHEET for critter "%s" — use it ONLY to lock the mascot identity: body, colors, palette, silhouette, proportions, and hard pixel-art style.
2. CROPPED SOURCE TILE — the healthy "%s" pose this animation starts from.

Create an animated flip-book deck of this EXACT character performing a specific workload activity.

Activity / theme: %s

Return one image that is a sprite sheet:
- exactly %d frames total
- arranged as one horizontal row, left to right
- each frame the same size
- no extra rows
- no text labels, no numbers, no borders, no UI, no legend, no watermark
- do not draw a checkerboard transparency grid
- transparent background with real alpha preferred
- preserve the exact character identity, palette, silhouette, proportions, alignment, and pixel-art style from the keyed sheet
- crisp pixel art only: hard square pixels, no blur, no soft painting, no 3D, no photorealism
- keep the sprite centered in each tile with consistent scale
- the frames should read as a smooth loop when played in order

CRITICAL: Each of the %d frames MUST be visually distinct — animate the pose, props, and effects per the recipe below. The character identity stays constant; the action changes.

Per-frame recipe (left to right, frames 1 through %d):
%s
Think like a sprite animator: lock the mascot from the keyed sheet, then render each frame per the recipe, all in one horizontal sheet.`,
		critterName, baseName, anim.Theme, n, n, n, recipe.String())
}

func sourceSheets(dir string) ([]string, error) {
	dirs, err := filepath.Glob(filepath.Join(dir, "*"))
	if err != nil {
		return nil, err
	}
	var out []string
	for _, d := range dirs {
		info, err := os.Stat(d)
		if err != nil || !info.IsDir() {
			continue
		}
		keyed := filepath.Join(d, "sprite-sheet-keyed.png")
		if _, err := os.Stat(keyed); err == nil {
			out = append(out, keyed)
			continue
		}
		base := filepath.Join(d, "sprite-sheet.png")
		if _, err := os.Stat(base); err == nil {
			out = append(out, base)
		}
	}
	return out, nil
}

func loadBaseFrames(path string) ([]byte, map[string][]byte, error) {
	fullSheet, err := os.ReadFile(path)
	if err != nil {
		return nil, nil, err
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, nil, err
	}
	defer f.Close()

	src, err := png.Decode(f)
	if err != nil {
		return nil, nil, err
	}
	b := src.Bounds()
	if b.Dx()%len(states) != 0 {
		return nil, nil, fmt.Errorf("width %d is not divisible by %d states", b.Dx(), len(states))
	}

	frameW := b.Dx() / len(states)
	out := make(map[string][]byte, len(states))
	for i, state := range states {
		rect := image.Rect(b.Min.X+i*frameW, b.Min.Y, b.Min.X+(i+1)*frameW, b.Max.Y)
		crop := image.NewNRGBA(image.Rect(0, 0, rect.Dx(), rect.Dy()))
		draw.Draw(crop, crop.Bounds(), src, rect.Min, draw.Src)

		var buf bytes.Buffer
		if err := png.Encode(&buf, crop); err != nil {
			return nil, nil, err
		}
		out[state] = buf.Bytes()
	}
	return fullSheet, out, nil
}

func generateDeck(ctx context.Context, model critterforge.ImageModel, critterName, state string, fullSheetPNG, statePNG []byte) ([]byte, error) {
	return model.GenerateSprite(ctx, deckPrompt(critterName, state), fullSheetPNG, statePNG)
}

func deckPrompt(critterName, state string) string {
	recipe := stateFrameRecipes[state]
	var frames strings.Builder
	for i, f := range recipe {
		fmt.Fprintf(&frames, "  Frame %d: %s\n", i+1, f)
	}

	stateBlock := critterforge.StatusBlock(state)
	if stateBlock == "" {
		// Fallback for legacy state names like "failed"; map to "error".
		stateBlock = critterforge.StatusBlock("error")
	}

	return fmt.Sprintf(`Two reference images are attached:
1. FULL KEYED STATUS SHEET for critter "%s" with 8 pod states in this order: Running, Pending, Completed, CrashLoopBackOff, BackOff, Terminating, Unknown, Error.
2. CROPPED SOURCE TILE for the exact "%s" / "%s" state.

Use the full keyed sheet to preserve the mascot identity, scale, alignment, palette, status vocabulary, and how Kubernetes pod states are visually encoded.
Use the cropped source tile as the exact state pose and expression to animate.

Your task is to create an animated flip-book deck from the cropped state tile.

Return one image that is a sprite sheet:
- exactly 8 frames total
- arranged as one horizontal row, left to right
- each frame is the same size
- no extra rows
- no text labels, no status words, no frame numbers, no borders, no legend, no UI, no watermark
- do not draw a checkerboard transparency grid
- transparent background with real alpha preferred
- preserve the exact character identity, species, palette, silhouette, proportions, alignment, and pixel-art style from the keyed sheet
- the 8 frames should read like a smooth looping flip book when played in order
- use crisp pixel art only: hard square pixels, no blur, no soft painting, no 3D, no photorealism
- keep the sprite centered in each tile with consistent scale

Authoritative status spec for this state — every frame must satisfy ALL of these. Read carefully and apply to every frame:

%s
Animation theme for this state: %s

CRITICAL: Each of the 8 frames MUST be visually distinct from the others. Do NOT draw the same character 8 times — animate the pose, expression, and added effects per frame, while staying inside the status spec above. The character identity stays constant; the pose/expression/effects change.

Per-frame recipe (left to right, frames 1 through 8). Use these as concrete pose deltas on top of the status spec:
%s
Think like a sprite animator: infer the mascot from the full keyed sheet, lock to the status spec, then render each of the 8 frames per the recipe, all in one horizontal sheet.`, critterName, stateDisplayName[state], state, stateBlock, stateAnimationBrief[state], frames.String())
}

func csvSet(s string) map[string]bool {
	out := map[string]bool{}
	for _, part := range strings.Split(s, ",") {
		if p := strings.TrimSpace(part); p != "" {
			out[p] = true
		}
	}
	return out
}
