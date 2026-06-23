package critterforge

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// Forge orchestrates sprite generation across many critters.
type Forge struct {
	opts Options
}

// New returns a Forge configured by opts. Returns an error if required
// options are missing.
func New(opts Options) (*Forge, error) {
	if opts.Model == nil {
		return nil, errors.New("critterforge: Options.Model is required")
	}
	if opts.OutputDir == "" {
		return nil, errors.New("critterforge: Options.OutputDir is required")
	}
	if opts.PromptVersion == "" {
		opts.PromptVersion = promptVersion
	}
	if opts.Concurrency <= 0 {
		opts.Concurrency = 4
	}
	if opts.Logger == nil {
		opts.Logger = noopLogger{}
	}
	if err := os.MkdirAll(opts.OutputDir, 0o755); err != nil {
		return nil, fmt.Errorf("critterforge: mkdir output: %w", err)
	}
	return &Forge{opts: opts}, nil
}

// GenerateAll reads inputPath as an InputManifest and generates every
// critter described there, in parallel up to Options.Concurrency. Failures
// for one critter do not stop the others; the first error encountered is
// returned at the end.
func (f *Forge) GenerateAll(ctx context.Context, inputPath string) error {
	return f.GenerateOnly(ctx, inputPath, nil)
}

// GenerateOnly is like GenerateAll but filters the manifest to only the
// named critters. An empty `only` list means "all critters". Returns an
// error if none of the requested names match an entry in the manifest.
//
// If the manifest declares a top-level style_reference path and the Forge
// was not constructed with Options.StyleReferencePNG already populated,
// the manifest's reference is loaded here.
func (f *Forge) GenerateOnly(ctx context.Context, inputPath string, only []string) error {
	input, err := loadInputManifest(inputPath)
	if err != nil {
		return err
	}
	if len(f.opts.StyleReferencePNG) == 0 && input.StyleReference != "" {
		data, err := os.ReadFile(input.StyleReference)
		if err != nil {
			return fmt.Errorf("load style reference %s: %w", input.StyleReference, err)
		}
		f.opts.StyleReferencePNG = data
	}
	specs := specsFromInput(f.opts.OutputDir, input)
	if len(only) > 0 {
		wanted := make(map[string]bool, len(only))
		for _, n := range only {
			wanted[n] = true
		}
		filtered := specs[:0]
		for _, s := range specs {
			if wanted[s.Name] {
				filtered = append(filtered, s)
			}
		}
		if len(filtered) == 0 {
			return fmt.Errorf("no critters in %s matched %v", inputPath, only)
		}
		specs = filtered
	}
	return f.GenerateSpecs(ctx, specs)
}

// GenerateSpecs is like GenerateAll but accepts already-built Specs. Useful
// for programmatic callers that don't want to round-trip through YAML.
func (f *Forge) GenerateSpecs(ctx context.Context, specs []Spec) error {
	manifest, err := loadOutputManifest(f.opts.OutputDir)
	if err != nil {
		return err
	}
	manifest.Model = f.opts.Model.ID()
	manifest.PromptVersion = f.opts.PromptVersion
	manifest.GeneratedAt = time.Now().UTC()

	sem := make(chan struct{}, f.opts.Concurrency)
	var wg sync.WaitGroup
	var mu sync.Mutex
	var firstErr error

	for _, spec := range specs {
		wg.Add(1)
		sem <- struct{}{}
		go func(spec Spec) {
			defer wg.Done()
			defer func() { <-sem }()

			f.opts.Logger.Logf("critter %s: starting", spec.Name)

			mu.Lock()
			prev := manifest.Critters[spec.Name]
			mu.Unlock()

			entry, err := f.generateOne(ctx, spec, prev)

			mu.Lock()
			defer mu.Unlock()
			// Always persist whatever we managed to generate, even on error,
			// so a resume run can skip the states that already succeeded.
			if len(entry.States) > 0 {
				manifest.Critters[spec.Name] = entry
				if werr := writeOutputManifest(f.opts.OutputDir, manifest); werr != nil {
					f.opts.Logger.Logf("critter %s: manifest write failed: %v", spec.Name, werr)
				}
			}
			if err != nil {
				f.opts.Logger.Logf("critter %s: FAILED: %v", spec.Name, err)
				if firstErr == nil {
					firstErr = fmt.Errorf("%s: %w", spec.Name, err)
				}
				return
			}
			f.opts.Logger.Logf("critter %s: done", spec.Name)
		}(spec)
	}
	wg.Wait()
	return firstErr
}

// generateOne ensures the base sprite exists, then walks the derived states.
func (f *Forge) generateOne(ctx context.Context, spec Spec, prev ManifestCritter) (ManifestCritter, error) {
	entry := ManifestCritter{
		Description:  spec.Description,
		Instructions: spec.Instructions,
		Sheet:        prev.Sheet, // never clobber the rendered keyed sheet
		States:       map[State]ManifestState{},
	}
	for k, v := range prev.States {
		entry.States[k] = v
	}

	critterDir := filepath.Join(f.opts.OutputDir, spec.Name)
	if err := os.MkdirAll(critterDir, 0o755); err != nil {
		return entry, fmt.Errorf("mkdir critter dir: %w", err)
	}
	// If a keyed sheet exists on disk and the manifest doesn't record it yet,
	// adopt it so the renderer can find it.
	if entry.Sheet == "" {
		if _, serr := os.Stat(filepath.Join(critterDir, SheetFilename)); serr == nil {
			entry.Sheet = filepath.Join(spec.Name, SheetFilename)
		}
	}

	basePNG, baseKey, err := f.ensureBase(ctx, spec, critterDir, entry.States[StateRunning])
	if err != nil {
		return entry, fmt.Errorf("base: %w", err)
	}
	entry.States[StateRunning] = ManifestState{
		Path:     filepath.Join(spec.Name, "running.png"),
		SHA256:   bytesSHA(basePNG),
		CacheKey: baseKey,
	}

	baseSHA := bytesSHA(basePNG)
	styleRefSHA := ""
	if len(f.opts.StyleReferencePNG) > 0 {
		styleRefSHA = bytesSHA(f.opts.StyleReferencePNG)
	}
	hasStyleRef := styleRefSHA != ""

	for _, state := range DerivedStates() {
		if err := ctx.Err(); err != nil {
			return entry, err
		}
		key := cacheKey(
			spec.Name,
			string(state),
			baseSHA,
			spec.Instructions,
			f.opts.PromptVersion,
			f.opts.Model.ID(),
			styleRefSHA,
		)
		statePath := filepath.Join(spec.Name, string(state)+".png")
		absPath := filepath.Join(f.opts.OutputDir, statePath)

		if !f.opts.Force {
			if existing, ok := entry.States[state]; ok && existing.CacheKey == key {
				if _, err := os.Stat(absPath); err == nil {
					f.opts.Logger.Logf("  %s/%s: cached", spec.Name, state)
					continue
				}
			}
		}

		f.opts.Logger.Logf("  %s/%s: generating", spec.Name, state)
		refs := make([][]byte, 0, 2)
		if hasStyleRef {
			refs = append(refs, f.opts.StyleReferencePNG)
		}
		refs = append(refs, basePNG)
		png, err := f.opts.Model.GenerateSprite(ctx, statePrompt(spec, state, hasStyleRef), refs...)
		if err != nil {
			return entry, fmt.Errorf("state %s: %w", state, err)
		}
		if err := os.WriteFile(absPath, png, 0o644); err != nil {
			return entry, fmt.Errorf("write %s: %w", state, err)
		}
		entry.States[state] = ManifestState{
			Path:     statePath,
			SHA256:   bytesSHA(png),
			CacheKey: key,
		}
	}
	return entry, nil
}

// ensureBase returns the canonical "running" PNG, generating it if needed.
// If the spec carries a ReferencePNG, that PNG is treated as canonical and
// Gemini is never called for the base. Otherwise the description drives a
// from-scratch generation that is then cached on disk.
func (f *Forge) ensureBase(ctx context.Context, spec Spec, critterDir string, prev ManifestState) ([]byte, string, error) {
	basePath := filepath.Join(critterDir, "running.png")

	if len(spec.ReferencePNG) > 0 {
		// Convert the reference (often smooth vector/cartoon art) into a true
		// pixel-art sprite, keeping the character's identity. Using the
		// reference raw would leave the running state un-pixelated while every
		// derived state is pixel art.
		key := cacheKey(
			spec.Name,
			"user-reference-pixelart",
			bytesSHA(spec.ReferencePNG),
			spec.Instructions,
			f.opts.PromptVersion,
			f.opts.Model.ID(),
		)
		if !f.opts.Force && prev.CacheKey == key {
			if data, err := os.ReadFile(basePath); err == nil {
				f.opts.Logger.Logf("  %s/running: cached (pixel-art from reference)", spec.Name)
				return data, key, nil
			}
		}
		f.opts.Logger.Logf("  %s/running: converting reference to pixel art", spec.Name)
		png, err := f.opts.Model.GenerateSprite(ctx, referenceBasePrompt(spec), spec.ReferencePNG)
		if err != nil {
			return nil, "", fmt.Errorf("convert reference to pixel art: %w", err)
		}
		if err := os.WriteFile(basePath, png, 0o644); err != nil {
			return nil, "", fmt.Errorf("write reference base: %w", err)
		}
		return png, key, nil
	}

	styleRefSHA := ""
	if len(f.opts.StyleReferencePNG) > 0 {
		styleRefSHA = bytesSHA(f.opts.StyleReferencePNG)
	}
	hasStyleRef := styleRefSHA != ""

	key := cacheKey(
		spec.Name,
		"generated",
		spec.Description,
		spec.Instructions,
		f.opts.PromptVersion,
		f.opts.Model.ID(),
		styleRefSHA,
	)
	if !f.opts.Force && prev.CacheKey == key {
		if data, err := os.ReadFile(basePath); err == nil {
			f.opts.Logger.Logf("  %s/running: cached", spec.Name)
			return data, key, nil
		}
	}
	f.opts.Logger.Logf("  %s/running: generating base", spec.Name)
	var refs [][]byte
	if hasStyleRef {
		refs = append(refs, f.opts.StyleReferencePNG)
	}
	png, err := f.opts.Model.GenerateSprite(ctx, basePrompt(spec, hasStyleRef), refs...)
	if err != nil {
		return nil, "", err
	}
	if err := os.WriteFile(basePath, png, 0o644); err != nil {
		return nil, "", fmt.Errorf("write base: %w", err)
	}
	return png, key, nil
}

// specsFromInput converts an InputManifest into Specs, resolving relative
// reference paths against the input manifest's directory.
func specsFromInput(_ string, input *InputManifest) []Spec {
	specs := make([]Spec, 0, len(input.Critters))
	for _, ic := range input.Critters {
		spec := Spec{
			Name:         ic.Name,
			Description:  ic.Description,
			Instructions: ic.Instructions,
		}
		if ic.Reference != "" {
			data, err := os.ReadFile(ic.Reference)
			if err == nil {
				spec.ReferencePNG = data
			}
		}
		specs = append(specs, spec)
	}
	return specs
}
