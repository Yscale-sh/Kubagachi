package critterforge

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// KeyedSheetSpec describes one critter's keyed-status sprite sheet.
// It carries the master-prompt fields that describe the mascot's identity.
type KeyedSheetSpec struct {
	// Name is the directory key (e.g. "claude-code"). Required.
	Name string

	// Mascot is the short mascot tagline (e.g. "cream robot cat"). Required.
	Mascot string

	// Personality describes how the critter feels/behaves
	// (e.g. "calm, intelligent, helpful, technical").
	Personality string

	// VisualRole describes its place in the system
	// (e.g. "AI/code assistant pod mascot").
	VisualRole string

	// VisualDesign is a bulleted list of body/feature constraints
	// (e.g. ["cream/off-white robot cat body", "dark outline", ...]).
	VisualDesign []string

	// Instructions is optional free-form steering appended verbatim.
	Instructions string

	// ReferencePNG, when set, is the canonical character art (e.g. the shared
	// Nori base) attached to the call so every status frame replicates it.
	ReferencePNG []byte
}

// SheetOptions configures keyed-sheet generation. Image size/quality
// are pinned by the keyed-sheet pipeline (8 frames in one row at
// 1536x1024 = 192x1024 per tile) and not exposed.
type SheetOptions struct {
	Model     ImageModel
	OutputDir string
	Force     bool
	Logger    Logger
}

// SheetFilename is the slicer-ready keyed sheet written under
// <OutputDir>/<critter>/ — real alpha, single horizontal row.
const SheetFilename = "sprite-sheet-keyed.png"

// keyedSheetFrames is the number of status frames per keyed sheet.
const keyedSheetFrames = 8

// SheetSize is the OpenAI image size used for keyed sheets.
// 1536x1024 fits 8 evenly-sized 192x1024 status tiles.
const SheetSize = "1536x1024"

// GenerateSheets walks specs and writes <OutputDir>/<name>/sprite-sheet-keyed.png
// for each one. Existing sheets are skipped unless Force is set.
func GenerateSheets(ctx context.Context, opts SheetOptions, specs []KeyedSheetSpec) error {
	if opts.Model == nil {
		return errors.New("critterforge: SheetOptions.Model is required")
	}
	if opts.OutputDir == "" {
		return errors.New("critterforge: SheetOptions.OutputDir is required")
	}
	if opts.Logger == nil {
		opts.Logger = noopLogger{}
	}
	if err := os.MkdirAll(opts.OutputDir, 0o755); err != nil {
		return fmt.Errorf("mkdir output: %w", err)
	}

	var firstErr error
	for _, spec := range specs {
		if err := ctx.Err(); err != nil {
			return err
		}
		if err := generateSheet(ctx, opts, spec); err != nil {
			opts.Logger.Logf("sheet %s: FAILED: %v", spec.Name, err)
			if firstErr == nil {
				firstErr = fmt.Errorf("%s: %w", spec.Name, err)
			}
			continue
		}
	}
	// Record each keyed-sheet path in manifest.json so the renderer (which
	// slices the sheet) can find it — without this the critter renders empty.
	if err := recordSheetsInManifest(opts.OutputDir, specs); err != nil {
		opts.Logger.Logf("sheet: manifest update failed: %v", err)
	}
	return firstErr
}

// recordSheetsInManifest sets each generated critter's Sheet path in
// manifest.json (creating the entry if absent), preserving everything else.
func recordSheetsInManifest(dir string, specs []KeyedSheetSpec) error {
	manifest, err := loadOutputManifest(dir)
	if err != nil {
		return err
	}
	changed := false
	for _, s := range specs {
		if _, err := os.Stat(filepath.Join(dir, s.Name, SheetFilename)); err != nil {
			continue
		}
		rel := filepath.Join(s.Name, SheetFilename)
		entry := manifest.Critters[s.Name]
		if entry.Sheet == rel {
			continue
		}
		entry.Sheet = rel
		if entry.States == nil {
			entry.States = map[State]ManifestState{}
		}
		manifest.Critters[s.Name] = entry
		changed = true
	}
	if changed {
		return writeOutputManifest(dir, manifest)
	}
	return nil
}

func generateSheet(ctx context.Context, opts SheetOptions, spec KeyedSheetSpec) error {
	if spec.Name == "" {
		return errors.New("spec.Name is required")
	}
	if spec.Mascot == "" {
		return errors.New("spec.Mascot is required")
	}
	critterDir := filepath.Join(opts.OutputDir, spec.Name)
	if err := os.MkdirAll(critterDir, 0o755); err != nil {
		return fmt.Errorf("mkdir critter dir: %w", err)
	}
	outPath := filepath.Join(critterDir, SheetFilename)
	if !opts.Force {
		if _, err := os.Stat(outPath); err == nil {
			opts.Logger.Logf("sheet %s: cached %s", spec.Name, outPath)
			return nil
		}
	}
	opts.Logger.Logf("sheet %s: generating %s", spec.Name, outPath)
	// Anchor the sheet to the canonical character art (the shared Nori base)
	// when the manifest supplies a reference, so every status frame is the same
	// cat instead of being re-imagined from text.
	var refs [][]byte
	if len(spec.ReferencePNG) > 0 {
		refs = append(refs, spec.ReferencePNG)
	}
	raw, err := opts.Model.GenerateSprite(ctx, sheetPrompt(spec, len(refs) > 0), refs...)
	if err != nil {
		return err
	}
	// Turn the raw grid/checkerboard model output into the slicer-ready sheet,
	// in memory — only the normalized keyed sheet is written to disk.
	keyed, err := NormalizeKeyedSheet(raw, keyedSheetFrames)
	if err != nil {
		return fmt.Errorf("normalize sheet: %w", err)
	}
	if err := os.WriteFile(outPath, keyed, 0o644); err != nil {
		return fmt.Errorf("write sheet: %w", err)
	}
	opts.Logger.Logf("sheet %s: normalized -> %s", spec.Name, outPath)
	return nil
}

// sheetPrompt renders the master keyed-status-sheet prompt for one critter.
// Mirrors the prompts the user was previously authoring by hand in ChatGPT.
func sheetPrompt(s KeyedSheetSpec, hasRef bool) string {
	var b strings.Builder
	if hasRef {
		b.WriteString("The attached image is the CANONICAL CHARACTER — a finished, crisp pixel-art sprite of this exact mascot. Every frame in the sheet MUST be this same character: identical body, costume, hat, props, colors, palette, and proportions, rendered in the SAME hard-pixel style. Re-pose it per the statuses below, but NEVER redesign it, soften it, or change its art style.\n\n")
	}
	fmt.Fprintf(&b, "Create a clean pixel-art sprite sheet of a cute %s mascot for a Kubernetes terminal TUI dashboard.\n\n", s.Mascot)
	b.WriteString("Service identity:\n")
	fmt.Fprintf(&b, "- Kubernetes service/pod name: %s\n", s.Name)
	fmt.Fprintf(&b, "- Mascot: %s\n", s.Mascot)
	if s.Personality != "" {
		fmt.Fprintf(&b, "- Personality: %s\n", s.Personality)
	}
	if s.VisualRole != "" {
		fmt.Fprintf(&b, "- Visual role: %s\n", s.VisualRole)
	}

	if len(s.VisualDesign) > 0 {
		b.WriteString("\nVisual design:\n")
		for _, line := range s.VisualDesign {
			fmt.Fprintf(&b, "- %s\n", line)
		}
	}

	b.WriteString(`
Sprite sheet requirements:
- transparent background with REAL alpha
- no checkerboard
- no background scene
- no UI frame
- no text labels
- no status words
- consistent sprite scale and alignment across every frame
- all frames use the same base mascot design
- hard pixel art only
- no gradients
- no anti-aliasing
- limited retro pixel palette
- readable as a game/UI sprite

Style:
- true retro pixel art
- cute Kubernetes TUI mascot style
- dark outline
- chunky readable silhouette
- full body visible
- centered in each tile
- same proportions in every frame

Layout:
- 8 frames total
- single horizontal row, left to right
- each frame occupies one evenly sized tile
- leave transparent padding around each sprite
- no labels under frames

Statuses (in this EXACT order, left to right). Each block describes the
visual goal, required pose, face, effects, color treatment, and a list of
things to avoid. Follow them literally:

`)
	b.WriteString(AllStatusBlocks())
	b.WriteString(`
Consistency rules:
- this is the SAME mascot in every frame
- only expression, pose, and minimal status effects change per the per-status block above
- keep the mascot identity recognizable
- keep scale and alignment consistent
- avoid gore or scary details
- keep it cute, readable, and technical

Output:
- one single sprite sheet PNG
- transparent background with real alpha
- no embedded checker pattern
`)

	if s.Instructions != "" {
		b.WriteString("\nAdditional instructions:\n- ")
		b.WriteString(s.Instructions)
		b.WriteString("\n")
	}
	return b.String()
}

// SheetSpecsFromInput pulls KeyedSheetSpecs out of an input manifest,
// skipping any critter that doesn't declare a Mascot (since Mascot is the
// only field required by the sheet pipeline).
func SheetSpecsFromInput(input *InputManifest) []KeyedSheetSpec {
	out := make([]KeyedSheetSpec, 0, len(input.Critters))
	for _, ic := range input.Critters {
		if ic.Mascot == "" {
			continue
		}
		spec := KeyedSheetSpec{
			Name:         ic.Name,
			Mascot:       ic.Mascot,
			Personality:  ic.Personality,
			VisualRole:   ic.VisualRole,
			VisualDesign: ic.VisualDesign,
			Instructions: ic.Instructions,
		}
		if ic.Reference != "" {
			if data, err := os.ReadFile(ic.Reference); err == nil {
				spec.ReferencePNG = data
			}
		}
		out = append(out, spec)
	}
	return out
}

// LoadInputManifest is the exported counterpart of loadInputManifest so
// CLI code in cmd/... can read the same YAML the forge uses.
func LoadInputManifest(path string) (*InputManifest, error) {
	return loadInputManifest(path)
}
