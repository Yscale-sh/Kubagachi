package critters

import (
	"encoding/json"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

var allStatuses = []string{
	"running", "pending", "crashloop", "backoff", "imagepull",
	"terminating", "unknown", "completed", "oomkilled", "failed",
}

func TestAssignIsDeterministic(t *testing.T) {
	for _, key := range []string{"default/web", "kube-system/coredns", "ns/pod-xyz"} {
		first := Assign(key)
		for i := 0; i < 50; i++ {
			if got := Assign(key); got != first {
				t.Fatalf("Assign(%q) not deterministic: %q vs %q", key, first, got)
			}
		}
	}
}

func TestAssignReturnsKnownCritter(t *testing.T) {
	known := map[string]bool{}
	for _, n := range Names() {
		known[n] = true
	}
	for _, key := range []string{"a/b", "c/d", "e/f", "g/h", "i/j", "k/l"} {
		if !known[Assign(key)] {
			t.Fatalf("Assign(%q) returned unknown critter", key)
		}
	}
}

func TestFrameHasFixedHeight(t *testing.T) {
	for _, name := range Names() {
		for _, status := range allStatuses {
			for tick := 0; tick < 12; tick++ {
				frame := Frame(name, status, tick)
				if got := strings.Count(frame, "\n") + 1; got != FrameHeight {
					t.Errorf("Frame(%q,%q,%d): %d lines, want %d", name, status, tick, got, FrameHeight)
				}
			}
		}
	}
}

func TestFrameUnknownCritterFallsBack(t *testing.T) {
	if Frame("not-a-critter", "running", 0) == "" {
		t.Fatal("Frame should fall back for unknown critter, got empty")
	}
}

func TestLoadPixelSpritesUsesSpriteSheetsOnly(t *testing.T) {
	defer resetPixelSpritesForTest()

	dir := t.TempDir()
	critterDir := filepath.Join(dir, "sheet-critter")
	if err := os.Mkdir(critterDir, 0o755); err != nil {
		t.Fatal(err)
	}

	writeTestSheet(t, filepath.Join(critterDir, "sprite-sheet.png"), color.NRGBA{R: 220, G: 10, B: 10, A: 255})
	if err := os.WriteFile(filepath.Join(critterDir, "running.png"), []byte("bad per-state image"), 0o644); err != nil {
		t.Fatal(err)
	}

	manifest := map[string]any{
		"critters": map[string]any{
			"sheet-critter": map[string]any{
				"sheet": "sheet-critter/sprite-sheet.png",
				"states": map[string]any{
					"running": map[string]any{
						"path": "sheet-critter/running.png",
					},
				},
			},
		},
	}
	data, err := json.Marshal(manifest)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "manifest.json"), data, 0o644); err != nil {
		t.Fatal(err)
	}

	if err := LoadPixelSprites(dir); err != nil {
		t.Fatal(err)
	}
	frame := Frame("sheet-critter", "running", 0)
	if strings.TrimSpace(frame) == "" {
		t.Fatal("Frame(sheet-critter, running) was empty")
	}
	resolved := ResolveInlineImages(frame)
	if resolved == frame {
		t.Fatal("ResolveInlineImages did not replace the image placeholder")
	}
	if !strings.Contains(resolved, string([]byte{0x1b})+"]1337;File=") {
		t.Fatal("resolved frame did not contain an inline image escape")
	}
}

func writeTestSheet(t *testing.T, path string, c color.NRGBA) {
	t.Helper()

	img := image.NewNRGBA(image.Rect(0, 0, len(sheetStatusOrder)*8, 8))
	for frame := range sheetStatusOrder {
		for y := 0; y < 8; y++ {
			for x := frame * 8; x < frame*8+8; x++ {
				img.SetNRGBA(x, y, c)
			}
		}
	}
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := png.Encode(f, img); err != nil {
		_ = f.Close()
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestLoadProjectPixelSpritesIncludesGeneratedSheets(t *testing.T) {
	defer resetPixelSpritesForTest()

	if _, err := os.Stat(filepath.Join("..", "..", "critters", "manifest.json")); err != nil {
		t.Skip("project critter manifest not present")
	}
	if err := LoadPixelSprites(filepath.Join("..", "..", "critters")); err != nil {
		t.Fatal(err)
	}

	for _, name := range []string{
		"worker",
		"job-processor",
		"email-sender",
		"payment-service",
		"reporter",
		"frontend",
		"websocket",
		"cache",
	} {
		if got := Frame(name, "running", 0); strings.TrimSpace(got) == "" {
			t.Fatalf("Frame(%s, running) was empty", name)
		}
		if got := Frame(name, "failed", 0); strings.TrimSpace(got) == "" {
			t.Fatalf("Frame(%s, failed) was empty", name)
		}
	}
}

func resetPixelSpritesForTest() {
	pixelMu.Lock()
	defer pixelMu.Unlock()
	pixelSprites = nil
	pixelImages = nil
	pixelNames = nil
}
