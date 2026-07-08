package critters

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
)

// PixelTargetSize is the width requested from the terminal inline image
// protocol. Keeping this <= podCardInnerW (18) lets the existing pod card
// layout absorb the sprite without resizing.
const PixelTargetSize = 16

// PixelTargetHeight is the terminal-cell height reserved for inline sprites.
const PixelTargetHeight = 8

// pixelManifest mirrors the shape of critterforge's manifest.json. We
// only need the path field; everything else (hashes, cache keys) is
// metadata for the generator, not the consumer.
type pixelManifest struct {
	Critters map[string]struct {
		Sheet  string `json:"sheet"`
		States map[string]struct {
			Path string `json:"path"`
		} `json:"states"`
	} `json:"critters"`
}

var (
	pixelMu      sync.RWMutex
	pixelSprites map[string]map[string]string // critter -> state -> inline-image layout placeholder
	pixelImages  map[string]string            // fixed-width placeholder -> inline image escape sequence
	pixelNames   []string                     // sorted critter names from the loaded manifest
)

// PixelLoaded reports whether a pixel sprite set has been loaded. When true,
// Names/Assign/Frame all switch over to the pixel set.
func PixelLoaded() bool {
	pixelMu.RLock()
	defer pixelMu.RUnlock()
	return pixelSprites != nil
}

// LoadPixelSprites reads critterforge's manifest.json from dir and prepares
// sprite sheets for direct terminal inline-image rendering. Individual
// per-state images are intentionally ignored; those generated assets are less
// reliable than the sheets.
func LoadPixelSprites(dir string) error {
	data, err := os.ReadFile(filepath.Join(dir, "manifest.json"))
	if err != nil {
		return fmt.Errorf("critters: read pixel manifest: %w", err)
	}
	var m pixelManifest
	if err := json.Unmarshal(data, &m); err != nil {
		return fmt.Errorf("critters: parse pixel manifest: %w", err)
	}
	if len(m.Critters) == 0 {
		return fmt.Errorf("critters: pixel manifest has no critters")
	}

	sprites := make(map[string]map[string]string, len(m.Critters))
	images := map[string]string{}
	names := make([]string, 0, len(m.Critters))
	nextImageID := 0
	for name, c := range m.Critters {
		if c.Sheet == "" {
			continue
		}

		states, sheetImages, err := loadInlineSheet(filepath.Join(dir, c.Sheet), nextImageID)
		if err != nil {
			return fmt.Errorf("critters: render %s sheet: %w", name, err)
		}
		for st, rendered := range states {
			nextImageID++
			images[strings.TrimSpace(rendered)] = sheetImages[st]
		}
		if len(states) == 0 {
			continue
		}
		sprites[name] = states
		names = append(names, name)
	}
	if len(names) == 0 {
		return fmt.Errorf("critters: no critters had any renderable states")
	}
	sort.Strings(names)

	pixelMu.Lock()
	pixelSprites = sprites
	pixelImages = images
	pixelNames = names
	pixelMu.Unlock()
	return nil
}

// pixelStatusFallback maps kubagachi statuses that don't appear in the
// critterforge sprite set onto the nearest equivalent that does. Lets a
// pod-status like imagepull or oomkilled render something sensible rather
// than falling all the way back to ASCII.
var pixelStatusFallback = map[string]string{
	"imagepull": "pending",
	"oomkilled": "crashloop",
	"failed":    "crashloop",
}

// pixelFrame returns the cached inline-image placeholder for (critter, status) if
// the pixel set is loaded and contains it. Returns "" otherwise so Frame
// can fall through to the ASCII renderer.
func pixelFrame(critter, status string) string {
	pixelMu.RLock()
	defer pixelMu.RUnlock()
	if pixelSprites == nil {
		return ""
	}
	states, ok := pixelSprites[critter]
	if !ok {
		return ""
	}
	if f, ok := states[status]; ok {
		return f
	}
	if alt, ok := pixelStatusFallback[status]; ok {
		if f, ok := states[alt]; ok {
			return f
		}
	}
	if f, ok := states["running"]; ok {
		return f
	}
	return ""
}

// ResolveInlineImages swaps fixed-width image placeholders for the terminal
// inline-image escape sequences prepared by LoadPixelSprites. Keeping the
// placeholders in the layout pass prevents lipgloss from wrapping or truncating
// raw OSC image data.
func ResolveInlineImages(s string) string {
	pixelMu.RLock()
	defer pixelMu.RUnlock()
	for placeholder, imageEscape := range pixelImages {
		s = strings.ReplaceAll(s, placeholder, imageEscape)
	}
	return s
}

var sheetStatusOrder = []string{
	"running",
	"pending",
	"completed",
	"crashloop",
	"backoff",
	"terminating",
	"unknown",
	"failed",
}

func loadInlineSheet(path string, startID int) (map[string]string, map[string]string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, nil, err
	}
	defer f.Close()
	img, err := png.Decode(f)
	if err != nil {
		return nil, nil, err
	}
	b := img.Bounds()
	if b.Dx()%len(sheetStatusOrder) != 0 {
		return nil, nil, fmt.Errorf("width %d is not divisible by %d frames", b.Dx(), len(sheetStatusOrder))
	}

	frameW := b.Dx() / len(sheetStatusOrder)
	states := make(map[string]string, len(sheetStatusOrder))
	images := make(map[string]string, len(sheetStatusOrder))
	for i, st := range sheetStatusOrder {
		x0 := b.Min.X + i*frameW
		frame := cropImage(img, image.Rect(x0, b.Min.Y, x0+frameW, b.Max.Y))
		var buf bytes.Buffer
		if err := png.Encode(&buf, frame); err != nil {
			return nil, nil, err
		}
		rendered, imageEscape := inlineImageFrame(fmt.Sprintf("%s.png", st), buf.Bytes(), startID+i)
		states[st] = rendered
		images[st] = imageEscape
	}
	return states, images, nil
}

func cropImage(src image.Image, rect image.Rectangle) *image.NRGBA {
	dst := image.NewNRGBA(image.Rect(0, 0, rect.Dx(), rect.Dy()))
	for y := rect.Min.Y; y < rect.Max.Y; y++ {
		for x := rect.Min.X; x < rect.Max.X; x++ {
			dst.Set(x-rect.Min.X, y-rect.Min.Y, src.At(x, y))
		}
	}
	return dst
}

func inlineImageFrame(name string, data []byte, id int) (string, string) {
	placeholder := fmt.Sprintf("@@KCI%09d@@", id)
	nameB64 := base64.StdEncoding.EncodeToString([]byte(name))
	payload := base64.StdEncoding.EncodeToString(data)
	imageEscape := fmt.Sprintf(
		"\x1b]1337;File=name=%s;size=%d;width=%dch;height=%dch;preserveAspectRatio=1;inline=1:%s\x07",
		nameB64, len(data), PixelTargetSize, PixelTargetHeight, payload,
	)

	lines := make([]string, PixelTargetHeight)
	lines[0] = placeholder
	for i := 1; i < len(lines); i++ {
		lines[i] = strings.Repeat(" ", PixelTargetSize)
	}
	return strings.Join(lines, "\n"), imageEscape
}
