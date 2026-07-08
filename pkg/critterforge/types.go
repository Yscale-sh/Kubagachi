// Package critterforge generates pixel-art critter sprites for kubagachi,
// one per Kubernetes pod state. It calls an image API once per
// (critter, state), then caches results on disk so re-runs are cheap.
//
// The canonical "running" sprite is the source of truth for a critter's
// identity; every derived-state sprite is generated with that PNG passed
// back as a reference image to keep the character visually consistent.
package critterforge

import "context"

// Spec describes a single critter to generate.
type Spec struct {
	// Name is the cache key and the directory the sprites are written under.
	Name string

	// Description is free-form text used when generating the base ("running")
	// sprite from scratch. Ignored if ReferencePNG is set.
	Description string

	// ReferencePNG, when non-nil, is used as the canonical base sprite
	// instead of asking the model to invent one. Lets users seed the pipeline
	// with their own pixel art.
	ReferencePNG []byte

	// Instructions is free-form steering appended to every state prompt
	// (base + all six derived). Use for cross-cutting constraints like
	// "keep the AI sign visible" or "only use a 4-color palette".
	Instructions string
}

// Options configures a Forge.
type Options struct {
	// Model is the image generator. Required. The interface exists so tests
	// can substitute a fake.
	Model ImageModel

	// OutputDir is where PNGs and manifest.json live. Created if missing.
	OutputDir string

	// PromptVersion is a cache-busting token mixed into every cache key.
	// Defaults to the package-internal prompt version; override if you've
	// forked the prompts and want your fork's cache to coexist with stock.
	PromptVersion string

	// StyleReferencePNG is an optional global style anchor. When non-nil,
	// it is prepended as the first reference image on every image call
	// (both base and derived-state) so generated sprites match its
	// pixel-art look. The CLI's --style-ref flag and the manifest's
	// top-level `style_reference` field both populate this.
	StyleReferencePNG []byte

	// Force re-generates every sprite even when the cache says it's fresh.
	Force bool

	// Concurrency caps the number of critters generated in parallel.
	// Within a single critter, derived states are still sequential because
	// each one is conditioned on the base. Defaults to 4.
	Concurrency int

	// Logger receives progress updates. nil means silent.
	Logger Logger
}

// ImageModel is the minimum surface critterforge needs from an image
// generator.
type ImageModel interface {
	// GenerateSprite returns PNG bytes for the given prompt. If references
	// are supplied, they are passed as input images for conditioning.
	GenerateSprite(ctx context.Context, prompt string, references ...[]byte) ([]byte, error)

	// ID returns a stable identifier for this model (e.g. the model string)
	// so it can participate in cache keys.
	ID() string
}

// Logger is the optional progress sink.
type Logger interface {
	Logf(format string, args ...any)
}

type noopLogger struct{}

func (noopLogger) Logf(string, ...any) {}
