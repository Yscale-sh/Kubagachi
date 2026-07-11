package critterforge

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"testing"
)

// synthSheet builds an opaque image that mimics Gemini's raw output: a baked
// two-tone checkerboard background with `cols`x`rows` solid critter blobs in a
// grid, each blob carrying an enclosed light "belly". An extra tiny detached
// particle is added near the first blob to exercise merge-to-expected. Returns
// PNG bytes.
func synthSheet(t *testing.T, cols, rows int, light, dark color.RGBA) []byte {
	t.Helper()
	const cell, blob, gap = 160, 90, 0 // cell size, blob size; gap implied by cell-blob
	w, h := cols*cell, rows*cell
	img := image.NewRGBA(image.Rect(0, 0, w, h))

	// Checkerboard background, fully opaque.
	const tile = 20
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			c := light
			if (x/tile+y/tile)%2 == 0 {
				c = dark
			}
			img.SetRGBA(x, y, c)
		}
	}

	outline := color.RGBA{40, 42, 48, 255}
	belly := color.RGBA{248, 248, 248, 255}
	for r := 0; r < rows; r++ {
		for cidx := 0; cidx < cols; cidx++ {
			cx := cidx*cell + cell/2
			cy := r*cell + cell/2
			// solid dark blob (the critter body) — dark + opaque
			for y := cy - blob/2; y < cy+blob/2; y++ {
				for x := cx - blob/2; x < cx+blob/2; x++ {
					img.SetRGBA(x, y, outline)
				}
			}
			// enclosed light belly — same color as light background, but walled
			// in by the dark blob, so flood-fill must NOT erase it.
			for y := cy - 12; y < cy+12; y++ {
				for x := cx - 12; x < cx+12; x++ {
					img.SetRGBA(x, y, belly)
				}
			}
		}
	}
	// detached particle near the first blob (a stray "sparkle").
	for y := cell/2 - 5; y < cell/2+5; y++ {
		for x := cell - 18; x < cell-8; x++ {
			img.SetRGBA(x, y, color.RGBA{90, 200, 120, 255})
		}
	}
	_ = gap

	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatalf("encode synth: %v", err)
	}
	return buf.Bytes()
}

func TestNormalizeKeyedSheet(t *testing.T) {
	light := color.RGBA{232, 232, 232, 255}
	dark := color.RGBA{150, 155, 165, 255}  // bluish dark checker, like a real deck
	raw := synthSheet(t, 4, 2, light, dark) // 8 blobs + 1 particle

	out, err := NormalizeKeyedSheet(raw, 8)
	if err != nil {
		t.Fatalf("NormalizeKeyedSheet: %v", err)
	}
	img, err := png.Decode(bytes.NewReader(out))
	if err != nil {
		t.Fatalf("decode normalized: %v", err)
	}
	nrgba, ok := img.(*image.NRGBA)
	if !ok {
		t.Fatalf("want *image.NRGBA, got %T", img)
	}
	b := nrgba.Bounds()
	w, h := b.Dx(), b.Dy()

	// 8 square tiles laid out in a single horizontal row -> width == 8*height.
	if w != 8*h {
		t.Errorf("want single row of 8 square tiles (w==8*h); got w=%d h=%d (w/h=%.2f)", w, h, float64(w)/float64(h))
	}

	// Corner must be fully transparent (checkerboard keyed out).
	if _, _, _, a := nrgba.At(1, 1).RGBA(); a != 0 {
		t.Errorf("corner alpha = %d, want 0 (background not keyed)", a>>8)
	}

	// Each tile's center must be opaque, and the enclosed belly must survive
	// (i.e., the blob center is opaque, not punched through to transparent).
	tile := h
	for i := 0; i < 8; i++ {
		cx := i*tile + tile/2
		cy := tile / 2
		if _, _, _, a := nrgba.At(cx, cy).RGBA(); a == 0 {
			t.Errorf("tile %d center (%d,%d) is transparent; belly/blob was erased", i, cx, cy)
		}
	}
}

func TestNormalizeGridSheet(t *testing.T) {
	light := color.RGBA{232, 232, 232, 255}
	dark := color.RGBA{150, 155, 165, 255}
	raw := synthSheet(t, 4, 2, light, dark) // 8 blobs in a 4x2 grid

	out, err := NormalizeGridSheet(raw, 4, 2)
	if err != nil {
		t.Fatalf("NormalizeGridSheet: %v", err)
	}
	img, err := png.Decode(bytes.NewReader(out))
	if err != nil {
		t.Fatalf("decode normalized: %v", err)
	}
	nrgba, ok := img.(*image.NRGBA)
	if !ok {
		t.Fatalf("want *image.NRGBA, got %T", img)
	}
	b := nrgba.Bounds()
	w, h := b.Dx(), b.Dy()

	// Square tiles in a 4x2 grid -> width == 2*height.
	if w != 2*h {
		t.Errorf("want 4x2 grid of square tiles (w==2*h); got w=%d h=%d", w, h)
	}
	if w%4 != 0 || h%2 != 0 {
		t.Errorf("dimensions %dx%d do not divide into a 4x2 grid", w, h)
	}

	// Corner must be fully transparent (checkerboard keyed out).
	if _, _, _, a := nrgba.At(1, 1).RGBA(); a != 0 {
		t.Errorf("corner alpha = %d, want 0 (background not keyed)", a>>8)
	}

	// Each cell's center must be opaque (one blob per cell, bellies intact).
	tile := w / 4
	for r := 0; r < 2; r++ {
		for c := 0; c < 4; c++ {
			cx := c*tile + tile/2
			cy := r*tile + tile/2
			if _, _, _, a := nrgba.At(cx, cy).RGBA(); a == 0 {
				t.Errorf("cell (%d,%d) center (%d,%d) is transparent", c, r, cx, cy)
			}
		}
	}
}

func TestNormalizeGridSheetRejectsEmptyInput(t *testing.T) {
	if _, err := NormalizeGridSheet([]byte("not a png"), 2, 2); err == nil {
		t.Error("undecodable input did not error")
	}
	if _, err := NormalizeGridSheet(nil, 0, 2); err == nil {
		t.Error("invalid grid shape did not error")
	}
}

func TestNormalizeExactGridSheetRejectsWrongLayout(t *testing.T) {
	raw := synthSheet(t, 2, 2, color.RGBA{232, 232, 232, 255}, color.RGBA{150, 155, 165, 255})
	if _, err := NormalizeExactGridSheet(raw, 4, 1); err == nil {
		t.Fatal("2x2 output accepted as a 4x1 animation strip")
	}
	if _, err := NormalizeExactGridSheet(raw, 4, 2); err == nil {
		t.Fatal("2x2 output accepted as a 4x2 asset grid")
	}
}

func TestNormalizeExactGridSheetAcceptsRequestedLayout(t *testing.T) {
	raw := synthSheet(t, 4, 2, color.RGBA{232, 232, 232, 255}, color.RGBA{150, 155, 165, 255})
	out, err := NormalizeExactGridSheet(raw, 4, 2)
	if err != nil {
		t.Fatalf("NormalizeExactGridSheet: %v", err)
	}
	img, err := png.Decode(bytes.NewReader(out))
	if err != nil {
		t.Fatalf("decode normalized exact grid: %v", err)
	}
	if got := float64(img.Bounds().Dx()) / float64(img.Bounds().Dy()); got != 2 {
		t.Fatalf("normalized aspect = %.2f, want 2", got)
	}
}

func TestResizeTileSheetUsesExactNearestNeighborCells(t *testing.T) {
	raw := synthSheet(t, 4, 2, color.RGBA{232, 232, 232, 255}, color.RGBA{150, 155, 165, 255})
	normalized, err := NormalizeGridSheet(raw, 4, 2)
	if err != nil {
		t.Fatalf("NormalizeGridSheet: %v", err)
	}
	resized, err := ResizeTileSheet(normalized, 4, 2, 16)
	if err != nil {
		t.Fatalf("ResizeTileSheet: %v", err)
	}
	img, err := png.Decode(bytes.NewReader(resized))
	if err != nil {
		t.Fatalf("decode resized sheet: %v", err)
	}
	if got, want := img.Bounds().Dx(), 64; got != want {
		t.Fatalf("width = %d, want %d", got, want)
	}
	if got, want := img.Bounds().Dy(), 32; got != want {
		t.Fatalf("height = %d, want %d", got, want)
	}
	if _, _, _, alpha := img.At(0, 0).RGBA(); alpha != 0 {
		t.Fatalf("corner alpha = %d, want transparent", alpha)
	}
}

func TestResizeTileSheetRejectsInvalidGeometry(t *testing.T) {
	if _, err := ResizeTileSheet(nil, 0, 1, 16); err == nil {
		t.Fatal("invalid grid did not error")
	}
	img := image.NewNRGBA(image.Rect(0, 0, 7, 5))
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatalf("encode source: %v", err)
	}
	if _, err := ResizeTileSheet(buf.Bytes(), 4, 1, 16); err == nil {
		t.Fatal("non-divisible source did not error")
	}
}

func TestNormalizeKeyedSheetLightChecker(t *testing.T) {
	// The keyed status sheet uses a lighter checker (~198); confirm the adaptive
	// keyer handles that brightness too.
	light := color.RGBA{255, 255, 255, 255}
	dark := color.RGBA{198, 198, 198, 255}
	raw := synthSheet(t, 4, 1, light, dark) // 4 blobs in a single row

	out, err := NormalizeKeyedSheet(raw, 4)
	if err != nil {
		t.Fatalf("NormalizeKeyedSheet: %v", err)
	}
	img, _ := png.Decode(bytes.NewReader(out))
	b := img.Bounds()
	if w, h := b.Dx(), b.Dy(); w != 4*h {
		t.Errorf("want single row of 4 tiles (w==4*h); got w=%d h=%d", w, h)
	}
	if _, _, _, a := img.At(1, 1).RGBA(); a != 0 {
		t.Errorf("corner alpha = %d, want 0", a>>8)
	}
}
