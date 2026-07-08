// Package sprites scans a critterforge-generated critters/ directory and
// describes the sprite sheets it finds. It is shared by the kubagachi web
// server and the critterview development gallery.
package sprites

import (
	"encoding/binary"
	"fmt"
	"image"
	_ "image/png"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
)

// States is the canonical state order used everywhere in the viewers.
// "failed" is accepted as an on-disk alias for "error".
var States = []string{
	"running",
	"pending",
	"completed",
	"crashloop",
	"backoff",
	"terminating",
	"unknown",
	"error",
}

var stateAliases = map[string][]string{
	"running":     {"running"},
	"pending":     {"pending"},
	"completed":   {"completed"},
	"crashloop":   {"crashloop"},
	"backoff":     {"backoff"},
	"terminating": {"terminating"},
	"unknown":     {"unknown"},
	"error":       {"error", "failed"},
}

// Info describes one critter's renderable artifacts.
type Info struct {
	Name          string             `json:"name"`
	KeyedURL      string             `json:"keyed_url,omitempty"`
	KeyedDim      *Dim               `json:"keyed_dim,omitempty"`
	KeyedHasAlpha bool               `json:"keyed_has_alpha"`
	Anim          map[string]AnimSrc `json:"anim,omitempty"` // state -> animation source
}

// Dim is a width/height pair.
type Dim struct {
	W int `json:"w"`
	H int `json:"h"`
}

// AnimSrc describes one per-state animation sheet.
type AnimSrc struct {
	URL      string        `json:"url"`
	W        int           `json:"w"`
	H        int           `json:"h"`
	Frames   int           `json:"frames"`
	HasAlpha bool          `json:"has_alpha"`
	Bounds   []FrameBounds `json:"bounds,omitempty"`
}

// FrameBounds is one detected frame's rectangle in the source sheet.
type FrameBounds struct {
	X0 int `json:"x0"`
	Y0 int `json:"y0"`
	X1 int `json:"x1"`
	Y1 int `json:"y1"`
}

// Scan walks dir and returns one Info per critter that has at least one
// renderable artifact. URLs are rooted at /critters/.
func Scan(dir string) ([]Info, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	out := []Info{}
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		name := e.Name()
		info := Info{
			Name: name,
			Anim: map[string]AnimSrc{},
		}
		critterDir := filepath.Join(dir, name)

		keyedPath := filepath.Join(critterDir, "sprite-sheet-keyed.png")
		if d, hasAlpha, ok := pngInfo(keyedPath); ok {
			info.KeyedURL = "/critters/" + name + "/sprite-sheet-keyed.png?v=" + mtimeTag(keyedPath)
			info.KeyedDim = &d
			info.KeyedHasAlpha = hasAlpha
		}

		for _, state := range States {
			for _, alias := range stateAliases[state] {
				p := filepath.Join(critterDir, "sprite-sheet-"+alias+".png")
				if d, hasAlpha, ok := pngInfo(p); ok {
					info.Anim[state] = AnimSrc{
						URL:      "/critters/" + name + "/sprite-sheet-" + alias + ".png?v=" + mtimeTag(p),
						W:        d.W,
						H:        d.H,
						Frames:   8,
						HasAlpha: hasAlpha,
						Bounds:   DetectFrameBounds(p, 8),
					}
					break
				}
			}
		}

		// Workload animation decks (bursting, scaling, …) live alongside the base
		// states but aren't in the canonical States list. Discover any extra
		// sprite-sheet-<name>.png and serve it under its own key so the UI can
		// play it when a pod's critterState names it.
		if extra, err := filepath.Glob(filepath.Join(critterDir, "sprite-sheet-*.png")); err == nil {
			for _, p := range extra {
				base := filepath.Base(p)
				if base == "sprite-sheet-keyed.png" {
					continue
				}
				stem := strings.TrimSuffix(strings.TrimPrefix(base, "sprite-sheet-"), ".png")
				if _, exists := info.Anim[stem]; exists {
					continue
				}
				if d, hasAlpha, ok := pngInfo(p); ok {
					info.Anim[stem] = AnimSrc{
						URL:      "/critters/" + name + "/" + base + "?v=" + mtimeTag(p),
						W:        d.W,
						H:        d.H,
						Frames:   8,
						HasAlpha: hasAlpha,
						Bounds:   DetectFrameBounds(p, 8),
					}
				}
			}
		}

		if info.KeyedURL == "" && len(info.Anim) == 0 {
			continue
		}
		out = append(out, info)
	}
	sort.Slice(out, func(i, j int) bool { return strings.ToLower(out[i].Name) < strings.ToLower(out[j].Name) })
	return out, nil
}

// LatestItem describes one PNG for the flat "most recent artifacts" list.
type LatestItem struct {
	Critter  string        `json:"critter"`
	State    string        `json:"state,omitempty"`
	Kind     string        `json:"kind"` // "anim" | "keyed" | "tile"
	URL      string        `json:"url"`
	Filename string        `json:"filename"`
	W        int           `json:"w"`
	H        int           `json:"h"`
	Frames   int           `json:"frames,omitempty"`
	HasAlpha bool          `json:"has_alpha"`
	Bounds   []FrameBounds `json:"bounds,omitempty"`
	ModUnix  int64         `json:"mod_unix"`
	ModISO   string        `json:"mod_iso"`
}

// ScanLatest walks the critters tree, classifies every PNG, and returns the
// limit most recently modified files.
func ScanLatest(dir string, limit int) ([]LatestItem, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	out := []LatestItem{}
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		name := e.Name()
		critterDir := filepath.Join(dir, name)
		files, err := os.ReadDir(critterDir)
		if err != nil {
			continue
		}
		for _, f := range files {
			if f.IsDir() || !strings.HasSuffix(f.Name(), ".png") {
				continue
			}
			full := filepath.Join(critterDir, f.Name())
			info, err := os.Stat(full)
			if err != nil {
				continue
			}
			d, hasAlpha, ok := pngInfo(full)
			if !ok {
				continue
			}
			kind, state, frames := classifyFilename(f.Name())
			var bounds []FrameBounds
			if kind == "anim" && frames > 0 {
				bounds = DetectFrameBounds(full, frames)
			}
			out = append(out, LatestItem{
				Critter:  name,
				State:    state,
				Kind:     kind,
				URL:      "/critters/" + name + "/" + f.Name() + "?v=" + mtimeTag(full),
				Filename: f.Name(),
				W:        d.W,
				H:        d.H,
				Frames:   frames,
				HasAlpha: hasAlpha,
				Bounds:   bounds,
				ModUnix:  info.ModTime().Unix(),
				ModISO:   info.ModTime().UTC().Format("2006-01-02T15:04:05Z"),
			})
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ModUnix > out[j].ModUnix })
	if len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}

func classifyFilename(name string) (kind, state string, frames int) {
	if name == "sprite-sheet-keyed.png" || name == "sprite-sheet.png" {
		return "keyed", "", 0
	}
	if strings.HasPrefix(name, "sprite-sheet-") && strings.HasSuffix(name, ".png") {
		stem := strings.TrimSuffix(strings.TrimPrefix(name, "sprite-sheet-"), ".png")
		if stem == "failed" {
			stem = "error"
		}
		return "anim", stem, 8
	}
	stem := strings.TrimSuffix(name, ".png")
	if stem == "failed" {
		stem = "error"
	}
	return "tile", stem, 0
}

// mtimeTag returns a short cache-busting token derived from the file's
// modification time.
func mtimeTag(path string) string {
	info, err := os.Stat(path)
	if err != nil {
		return "0"
	}
	return fmt.Sprintf("%d", info.ModTime().UnixNano())
}

// boundsCache memoizes column-scan frame detection by (path, mtimeNano).
var (
	boundsCacheMu sync.RWMutex
	boundsCache   = map[string][]FrameBounds{}
)

// DetectFrameBounds returns one rect per detected frame in the PNG at path.
// Caller passes the expected frame count for fallback / sanity checking.
func DetectFrameBounds(path string, expected int) []FrameBounds {
	info, err := os.Stat(path)
	if err != nil {
		return nil
	}
	key := fmt.Sprintf("%s:%d", path, info.ModTime().UnixNano())

	boundsCacheMu.RLock()
	if v, ok := boundsCache[key]; ok {
		boundsCacheMu.RUnlock()
		return v
	}
	boundsCacheMu.RUnlock()

	v := computeFrameBounds(path, expected)
	if v != nil {
		boundsCacheMu.Lock()
		boundsCache[key] = v
		boundsCacheMu.Unlock()
	}
	return v
}

func computeFrameBounds(path string, expected int) []FrameBounds {
	f, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer f.Close()
	src, _, err := image.Decode(f)
	if err != nil {
		return nil
	}
	b := src.Bounds()
	w := b.Dx()
	h := b.Dy()
	if w == 0 || h == 0 {
		return nil
	}

	// Per-column accounting: a column needs enough opaque pixels to count as
	// "inside a frame" so stray glitter doesn't bridge two real frames.
	const alphaMin = 24
	colCount := make([]int, w)
	colY0 := make([]int, w)
	colY1 := make([]int, w)
	for i := range colY0 {
		colY0[i] = h
		colY1[i] = -1
	}

	mark := func(x, y int) {
		colCount[x]++
		if y < colY0[x] {
			colY0[x] = y
		}
		if y > colY1[x] {
			colY1[x] = y
		}
	}

	switch img := src.(type) {
	case *image.NRGBA:
		for y := 0; y < h; y++ {
			row := y * img.Stride
			for x := 0; x < w; x++ {
				if img.Pix[row+x*4+3] > alphaMin {
					mark(x, y)
				}
			}
		}
	case *image.RGBA:
		for y := 0; y < h; y++ {
			row := y * img.Stride
			for x := 0; x < w; x++ {
				if img.Pix[row+x*4+3] > alphaMin {
					mark(x, y)
				}
			}
		}
	default:
		for y := 0; y < h; y++ {
			for x := 0; x < w; x++ {
				_, _, _, a := src.At(b.Min.X+x, b.Min.Y+y).RGBA()
				if a>>8 > alphaMin {
					mark(x, y)
				}
			}
		}
	}

	densityMin := h / 25
	if densityMin < 6 {
		densityMin = 6
	}
	colHasOpaque := make([]bool, w)
	for x := 0; x < w; x++ {
		if colCount[x] >= densityMin {
			colHasOpaque[x] = true
		}
	}

	gapTolerance := w / (expected * 12)
	if gapTolerance < 2 {
		gapTolerance = 2
	}
	var clusters []FrameBounds
	var cur *FrameBounds
	gapRun := 0
	for x := 0; x < w; x++ {
		if colHasOpaque[x] {
			if cur == nil {
				cur = &FrameBounds{X0: x, X1: x, Y0: colY0[x], Y1: colY1[x]}
			} else {
				cur.X1 = x
				if colY0[x] < cur.Y0 {
					cur.Y0 = colY0[x]
				}
				if colY1[x] > cur.Y1 {
					cur.Y1 = colY1[x]
				}
			}
			gapRun = 0
		} else if cur != nil {
			gapRun++
			if gapRun > gapTolerance {
				cur.X1++ // exclusive
				cur.Y1++
				clusters = append(clusters, *cur)
				cur = nil
				gapRun = 0
			}
		}
	}
	if cur != nil {
		cur.X1++
		cur.Y1++
		clusters = append(clusters, *cur)
	}
	if len(clusters) == 0 {
		return nil
	}
	return clusters
}

// pngInfo returns the PNG's width/height plus whether it carries an alpha
// channel, by reading the IHDR chunk directly.
func pngInfo(path string) (Dim, bool, bool) {
	f, err := os.Open(path)
	if err != nil {
		return Dim{}, false, false
	}
	defer f.Close()

	var head [26]byte
	if _, err := io.ReadFull(f, head[:]); err != nil {
		return Dim{}, false, false
	}
	if string(head[0:8]) != "\x89PNG\r\n\x1a\n" {
		return Dim{}, false, false
	}
	if string(head[12:16]) != "IHDR" {
		return Dim{}, false, false
	}
	w := int(binary.BigEndian.Uint32(head[16:20]))
	h := int(binary.BigEndian.Uint32(head[20:24]))
	colorType := head[25]
	hasAlpha := colorType == 4 || colorType == 6
	return Dim{W: w, H: h}, hasAlpha, true
}
