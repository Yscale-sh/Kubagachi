package critterforge

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"gopkg.in/yaml.v3"
)

// InputManifest is the YAML format users author to drive generation.
type InputManifest struct {
	// StyleReference is an optional path to a global style-reference PNG.
	// When set, the image is attached to every image call as a style
	// anchor. CLI's --style-ref flag overrides this when both are present.
	StyleReference string         `yaml:"style_reference,omitempty"`
	Critters       []InputCritter `yaml:"critters"`
}

// InputCritter is one critter entry in the input manifest. Either
// Description or Reference must be set; Instructions is always optional.
//
// The Mascot/Personality/VisualRole/VisualDesign fields are only used by
// the `sheet` pipeline (keyed status sprite sheet). The per-state forge
// (`generate`) still keys off Description/Reference/Instructions.
type InputCritter struct {
	Name         string `yaml:"name"`
	Description  string `yaml:"description,omitempty"`
	Reference    string `yaml:"reference,omitempty"` // path to a user-supplied PNG
	Instructions string `yaml:"instructions,omitempty"`

	Mascot       string   `yaml:"mascot,omitempty"`
	Personality  string   `yaml:"personality,omitempty"`
	VisualRole   string   `yaml:"visual_role,omitempty"`
	VisualDesign []string `yaml:"visual_design,omitempty"`

	// Animations are workload-specific decks beyond the eight base health
	// states (e.g. yscale's "bursting"). spriteanim renders each into
	// sprite-sheet-<state>.png from the critter's healthy base pose.
	Animations []InputAnimation `yaml:"animations,omitempty"`
}

// InputAnimation describes one workload animation deck. The State name is what
// kubagachi sets as a pod's CritterState to play it; Theme + Frames drive the
// generation prompt; Base names the keyed-sheet status tile the action animates
// from (default "running").
type InputAnimation struct {
	State   string   `yaml:"state"`
	Project string   `yaml:"project,omitempty"`
	Theme   string   `yaml:"theme"`
	Base    string   `yaml:"base,omitempty"`
	Frames  []string `yaml:"frames"`
}

// AnimationsByCritter returns a name→animations map for an input manifest, so
// generators can look up a critter's workload decks.
func AnimationsByCritter(m *InputManifest) map[string][]InputAnimation {
	out := make(map[string][]InputAnimation, len(m.Critters))
	for _, ic := range m.Critters {
		if len(ic.Animations) > 0 {
			out[ic.Name] = ic.Animations
		}
	}
	return out
}

// OutputManifest is the JSON written alongside generated PNGs. It records
// what was generated, with what prompts and model, so subsequent runs can
// decide whether to skip or regenerate each sprite.
type OutputManifest struct {
	Version       int                        `json:"version"`
	Model         string                     `json:"model"`
	PromptVersion string                     `json:"prompt_version"`
	GeneratedAt   time.Time                  `json:"generated_at"`
	Critters      map[string]ManifestCritter `json:"critters"`
}

// ManifestCritter is the per-critter manifest entry.
type ManifestCritter struct {
	Description  string `json:"description,omitempty"`
	Instructions string `json:"instructions,omitempty"`
	// Sheet is the keyed-status sprite-sheet path (relative to the manifest
	// dir) that the renderer slices. Preserved across runs so a tile-level
	// regeneration never clobbers the sheet the TUI/web actually render from.
	Sheet  string                  `json:"sheet,omitempty"`
	States map[State]ManifestState `json:"states"`
}

// ManifestState records one generated sprite. Path is relative to the
// manifest's directory.
type ManifestState struct {
	Path     string `json:"path"`
	SHA256   string `json:"sha256"`
	CacheKey string `json:"cache_key"`
}

const manifestFilename = "manifest.json"

func loadInputManifest(path string) (*InputManifest, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read input manifest: %w", err)
	}
	var m InputManifest
	if err := yaml.Unmarshal(data, &m); err != nil {
		return nil, fmt.Errorf("parse input manifest: %w", err)
	}
	if len(m.Critters) == 0 {
		return nil, errors.New("input manifest has no critters")
	}
	return &m, nil
}

func loadOutputManifest(dir string) (*OutputManifest, error) {
	path := filepath.Join(dir, manifestFilename)
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return &OutputManifest{
			Version:  1,
			Critters: map[string]ManifestCritter{},
		}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read output manifest: %w", err)
	}
	var m OutputManifest
	if err := json.Unmarshal(data, &m); err != nil {
		return nil, fmt.Errorf("parse output manifest: %w", err)
	}
	if m.Critters == nil {
		m.Critters = map[string]ManifestCritter{}
	}
	return &m, nil
}

func writeOutputManifest(dir string, m *OutputManifest) error {
	data, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, manifestFilename), data, 0644)
}
