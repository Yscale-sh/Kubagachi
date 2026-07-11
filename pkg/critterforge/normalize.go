package critterforge

import (
	"bytes"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	_ "image/jpeg" // decode Gemini's JPEG output
	"image/png"
	"sort"
)

// Gemini's image-preview model returns an opaque JPEG with a baked white +
// light-gray checkerboard standing in for "transparent background", and it
// free-arranges multi-frame sheets into a grid instead of the single row the
// slicer expects. NormalizeKeyedSheet turns that raw output into the format the
// renderer consumes: a real-alpha PNG with every frame in one horizontal row.
//
// The two steps:
//
//  1. Flood-fill alpha. The background is light + near-neutral AND connected to
//     the image border. We BFS from the border over background-candidate pixels
//     and clear their alpha. Enclosed light areas — Nori's white belly, sparkle
//     highlights inside the body outline — are unreachable from the border, so
//     they survive. A plain color-key would punch holes in them.
//  2. Grid reflow. We detect frame blobs (row bands, then column clusters within
//     each band), read them row-major, tight-crop each to its opaque bounds, and
//     re-lay them into a single evenly-tiled row with transparent padding.
//
// frames is the expected frame count, used only for a sanity log; detection is
// data-driven and tolerates 7 or 8.
func NormalizeKeyedSheet(raw []byte, frames int) ([]byte, error) {
	src, _, err := image.Decode(bytes.NewReader(raw))
	if err != nil {
		return nil, fmt.Errorf("decode raw sheet: %w", err)
	}
	rgba := toRGBA(src)
	alpha := buildAlpha(rgba)
	detected := detectGridFrames(rgba, alpha, frames)
	if len(detected) == 0 {
		return nil, fmt.Errorf("normalize: no frames detected")
	}
	row := reflowToRow(rgba, alpha, detected)
	var buf bytes.Buffer
	if err := png.Encode(&buf, row); err != nil {
		return nil, fmt.Errorf("encode normalized sheet: %w", err)
	}
	return buf.Bytes(), nil
}

// NormalizeGridSheet is NormalizeKeyedSheet's counterpart for asset grids: it
// keys out the baked background the same way, then slices the opaque bounding
// box into an exact columns x rows grid (row-major) and re-lays each cell's
// content into evenly sized square tiles with transparent padding. Cell
// positions are preserved — an empty cell stays an empty tile — so item N
// always lives at grid position N.
func NormalizeGridSheet(raw []byte, columns, rows int) ([]byte, error) {
	if columns < 1 || rows < 1 {
		return nil, fmt.Errorf("normalize grid: invalid grid %dx%d", columns, rows)
	}
	src, _, err := image.Decode(bytes.NewReader(raw))
	if err != nil {
		return nil, fmt.Errorf("decode raw grid: %w", err)
	}
	rgba := toRGBA(src)
	alpha := buildAlpha(rgba)
	w := rgba.Bounds().Dx()
	h := rgba.Bounds().Dy()

	minX, minY, maxX, maxY := w, h, -1, -1
	for y := 0; y < h; y++ {
		base := y * w
		for x := 0; x < w; x++ {
			if alpha[base+x] > 0 {
				if x < minX {
					minX = x
				}
				if x > maxX {
					maxX = x
				}
				if y < minY {
					minY = y
				}
				if y > maxY {
					maxY = y
				}
			}
		}
	}
	if maxX < 0 {
		return nil, fmt.Errorf("normalize grid: no content detected")
	}
	bw, bh := maxX+1-minX, maxY+1-minY

	cells := make([]image.Rectangle, 0, columns*rows)
	for r := 0; r < rows; r++ {
		ys := minY + bh*r/rows
		ye := minY + bh*(r+1)/rows
		for c := 0; c < columns; c++ {
			xs := minX + bw*c/columns
			xe := minX + bw*(c+1)/columns
			cells = append(cells, image.Rect(xs, ys, xe, ye))
		}
	}
	out := reflowToGrid(rgba, alpha, cells, columns, rows)
	var buf bytes.Buffer
	if err := png.Encode(&buf, out); err != nil {
		return nil, fmt.Errorf("encode normalized grid: %w", err)
	}
	return buf.Bytes(), nil
}

// NormalizeExactGridSheet validates that the model honored an explicit grid
// before normalizing it. Unlike the legacy Kubagachi normalizer, this rejects
// wrong-layout output rather than manufacturing the requested count by slicing
// a different composition.
func NormalizeExactGridSheet(raw []byte, columns, rows int) ([]byte, error) {
	if columns < 1 || rows < 1 {
		return nil, fmt.Errorf("normalize exact grid: invalid grid %dx%d", columns, rows)
	}
	src, _, err := image.Decode(bytes.NewReader(raw))
	if err != nil {
		return nil, fmt.Errorf("decode exact grid: %w", err)
	}
	rgba := toRGBA(src)
	alpha := buildAlpha(rgba)
	w, h := rgba.Bounds().Dx(), rgba.Bounds().Dy()
	minX, minY, maxX, maxY := w, h, -1, -1
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			if alpha[y*w+x] == 0 {
				continue
			}
			minX, minY = min(minX, x), min(minY, y)
			maxX, maxY = max(maxX, x), max(maxY, y)
		}
	}
	if maxX < 0 {
		return nil, fmt.Errorf("normalize exact grid: no content detected")
	}
	bw, bh := maxX+1-minX, maxY+1-minY
	expectedAspect := float64(columns) / float64(rows)
	actualAspect := float64(bw) / float64(bh)
	if actualAspect < expectedAspect*0.6 || actualAspect > expectedAspect*1.67 {
		return nil, fmt.Errorf("normalize exact grid: content aspect %.2f does not match %dx%d layout", actualAspect, columns, rows)
	}

	for row := 0; row < rows; row++ {
		ys, ye := minY+bh*row/rows, minY+bh*(row+1)/rows
		for column := 0; column < columns; column++ {
			xs, xe := minX+bw*column/columns, minX+bw*(column+1)/columns
			opaque := 0
			for y := ys; y < ye; y++ {
				for x := xs; x < xe; x++ {
					if alpha[y*w+x] > 0 {
						opaque++
					}
				}
			}
			minimum := max(4, (xe-xs)*(ye-ys)/500)
			if opaque < minimum {
				return nil, fmt.Errorf("normalize exact grid: cell %d,%d is empty or incomplete", column, row)
			}
		}
	}

	// Explicit sheets promise transparent gutters. Significant foreground on
	// an internal divider usually means one large pose was split into cells.
	for column := 1; column < columns; column++ {
		x := minX + bw*column/columns
		touches := 0
		for y := minY; y <= maxY; y++ {
			for _, sampleX := range []int{x - 1, x} {
				if sampleX >= 0 && sampleX < w && alpha[y*w+sampleX] > 0 {
					touches++
				}
			}
		}
		if touches > max(2, bh/50) {
			return nil, fmt.Errorf("normalize exact grid: content crosses vertical divider %d", column)
		}
	}
	for row := 1; row < rows; row++ {
		y := minY + bh*row/rows
		touches := 0
		for x := minX; x <= maxX; x++ {
			for _, sampleY := range []int{y - 1, y} {
				if sampleY >= 0 && sampleY < h && alpha[sampleY*w+x] > 0 {
					touches++
				}
			}
		}
		if touches > max(2, bw/50) {
			return nil, fmt.Errorf("normalize exact grid: content crosses horizontal divider %d", row)
		}
	}
	return NormalizeGridSheet(raw, columns, rows)
}

// ResizeTileSheet resamples an already-normalized tile sheet to exact square
// cell dimensions using nearest-neighbor sampling so hard pixel edges and
// alpha are preserved. The source sheet must divide evenly into its grid.
func ResizeTileSheet(raw []byte, columns, rows, tileSize int) ([]byte, error) {
	if columns < 1 || rows < 1 || tileSize < 1 {
		return nil, fmt.Errorf("resize tile sheet: invalid grid %dx%d at %dpx", columns, rows, tileSize)
	}
	src, _, err := image.Decode(bytes.NewReader(raw))
	if err != nil {
		return nil, fmt.Errorf("decode tile sheet: %w", err)
	}
	bounds := src.Bounds()
	if bounds.Dx()%columns != 0 || bounds.Dy()%rows != 0 {
		return nil, fmt.Errorf("resize tile sheet: source %dx%d does not divide into %dx%d", bounds.Dx(), bounds.Dy(), columns, rows)
	}
	sourceTileW := bounds.Dx() / columns
	sourceTileH := bounds.Dy() / rows
	out := image.NewNRGBA(image.Rect(0, 0, columns*tileSize, rows*tileSize))
	for row := 0; row < rows; row++ {
		for column := 0; column < columns; column++ {
			for y := 0; y < tileSize; y++ {
				sy := bounds.Min.Y + row*sourceTileH + y*sourceTileH/tileSize
				for x := 0; x < tileSize; x++ {
					sx := bounds.Min.X + column*sourceTileW + x*sourceTileW/tileSize
					out.SetNRGBA(column*tileSize+x, row*tileSize+y, color.NRGBAModel.Convert(src.At(sx, sy)).(color.NRGBA))
				}
			}
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, out); err != nil {
		return nil, fmt.Errorf("encode resized tile sheet: %w", err)
	}
	return buf.Bytes(), nil
}

func toRGBA(src image.Image) *image.RGBA {
	if r, ok := src.(*image.RGBA); ok {
		return r
	}
	b := src.Bounds()
	dst := image.NewRGBA(image.Rect(0, 0, b.Dx(), b.Dy()))
	draw.Draw(dst, dst.Bounds(), src, b.Min, draw.Src)
	return dst
}

// neutralSpread is the max channel spread for a pixel to count as part of the
// (always near-gray) baked checkerboard. Nori's gray fur is also neutral, but
// it's protected by flood-fill connectivity, not by this test.
const neutralSpread = 30

func channelMinMax(r, g, b uint8) (mn, mx uint8) {
	mn, mx = r, r
	if g < mn {
		mn = g
	}
	if b < mn {
		mn = b
	}
	if g > mx {
		mx = g
	}
	if b > mx {
		mx = b
	}
	return mn, mx
}

// learnDarkBackgroundLevel inspects the outer border ring — which is always
// background — and returns the darkest checker level (the 5th percentile of the
// min-channel among neutral border pixels). Gemini uses a different checker
// brightness per generation (the keyed sheet's is ~198, an animation deck's can
// be ~150), so the keyer adapts instead of hard-coding a threshold.
func learnDarkBackgroundLevel(img *image.RGBA) int {
	w := img.Bounds().Dx()
	h := img.Bounds().Dy()
	const ring = 4
	var mins []int
	consider := func(x, y int) {
		o := img.PixOffset(x, y)
		mn, mx := channelMinMax(img.Pix[o], img.Pix[o+1], img.Pix[o+2])
		if int(mx)-int(mn) <= neutralSpread {
			mins = append(mins, int(mn))
		}
	}
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			if x < ring || x >= w-ring || y < ring || y >= h-ring {
				consider(x, y)
			}
		}
	}
	if len(mins) == 0 {
		return 170 // no neutral border; fall back to a light-only key
	}
	sort.Ints(mins)
	return mins[len(mins)*5/100]
}

// buildAlpha returns a per-pixel alpha mask (0 = background, 255 = subject).
// If the decoded image already carries real transparency (a future provider
// might), that alpha is trusted directly. Otherwise the baked checkerboard is
// keyed out by flooding from the border over background-candidate pixels
// (near-neutral AND at least as light as the learned dark checker level).
func buildAlpha(img *image.RGBA) []uint8 {
	w := img.Bounds().Dx()
	h := img.Bounds().Dy()
	alpha := make([]uint8, w*h)

	// Honor real alpha if the source actually has some transparency.
	transparent := 0
	for i := 0; i < w*h; i++ {
		a := img.Pix[i*4+3]
		alpha[i] = a
		if a < 16 {
			transparent++
		}
	}
	if transparent > w*h/50 { // >2% transparent → trust the source's alpha
		return alpha
	}
	for i := range alpha {
		alpha[i] = 255
	}

	darkLevel := learnDarkBackgroundLevel(img)
	const margin = 14
	floor := darkLevel - margin

	visited := make([]bool, w*h)
	stack := make([]int, 0, w*h/4)

	isBG := func(x, y int) bool {
		o := img.PixOffset(x, y)
		mn, mx := channelMinMax(img.Pix[o], img.Pix[o+1], img.Pix[o+2])
		return int(mx)-int(mn) <= neutralSpread && int(mn) >= floor
	}
	push := func(x, y int) {
		idx := y*w + x
		if visited[idx] {
			return
		}
		if !isBG(x, y) {
			return
		}
		visited[idx] = true
		alpha[idx] = 0
		stack = append(stack, idx)
	}

	for x := 0; x < w; x++ {
		push(x, 0)
		push(x, h-1)
	}
	for y := 0; y < h; y++ {
		push(0, y)
		push(w-1, y)
	}
	for len(stack) > 0 {
		idx := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		x, y := idx%w, idx/w
		if x > 0 {
			push(x-1, y)
		}
		if x < w-1 {
			push(x+1, y)
		}
		if y > 0 {
			push(x, y-1)
		}
		if y < h-1 {
			push(x, y+1)
		}
	}
	return alpha
}

// detectGridFrames finds frame rectangles by banding opaque rows, then
// clustering opaque columns within each band. Frames are returned row-major
// (top-to-bottom, left-to-right) so the natural reading order matches the
// status order the sheet prompt requested.
func detectGridFrames(img *image.RGBA, alpha []uint8, expected int) []image.Rectangle {
	w := img.Bounds().Dx()
	h := img.Bounds().Dy()

	rowOpaque := make([]bool, h)
	rowThresh := w / 200
	for y := 0; y < h; y++ {
		c := 0
		base := y * w
		for x := 0; x < w; x++ {
			if alpha[base+x] > 0 {
				c++
			}
		}
		rowOpaque[y] = c > rowThresh
	}
	bands := runs(rowOpaque, h/30)

	type cell struct {
		band int
		col  span
		y    span
	}
	var cells []cell
	for bi, band := range bands {
		bandH := band.end - band.start
		colOpaque := make([]bool, w)
		colThresh := bandH / 40
		for x := 0; x < w; x++ {
			c := 0
			for y := band.start; y < band.end; y++ {
				if alpha[y*w+x] > 0 {
					c++
				}
			}
			colOpaque[x] = c > colThresh
		}
		for _, col := range runs(colOpaque, w/40) {
			cells = append(cells, cell{band: bi, col: col, y: band})
		}
	}

	// Detached effect particles (sparkles, "?", "Z") and frames split by a
	// transparent gap show up as extra column clusters. We know how many frames
	// there should be, so repeatedly fold the narrowest cluster into its nearest
	// same-band neighbor until the count matches.
	for expected > 0 && len(cells) > expected {
		narrow := -1
		for i, c := range cells {
			if narrow < 0 || (c.col.end-c.col.start) < (cells[narrow].col.end-cells[narrow].col.start) {
				narrow = i
			}
		}
		target, gap := -1, 1<<30
		for j, c := range cells {
			if j == narrow || c.band != cells[narrow].band {
				continue
			}
			d := cells[narrow].col.start - c.col.end
			if d < 0 {
				d = c.col.start - cells[narrow].col.end
			}
			if d < gap {
				gap, target = d, j
			}
		}
		if target < 0 {
			break // nothing to merge into (lone cluster in its band)
		}
		cells[target].col.start = min(cells[target].col.start, cells[narrow].col.start)
		cells[target].col.end = max(cells[target].col.end, cells[narrow].col.end)
		cells = append(cells[:narrow], cells[narrow+1:]...)
	}

	// Grid-recovery fallback. The prompt asks for `expected` equal frames, but
	// detection can land on the wrong count several ways: a silhouette state whose
	// frames touch edge-to-edge (undercount), or a scatter state — dissolve
	// particles, floating "?"/"Z" glitch marks — that splits into extra bands and
	// blobs. Critically, the model sometimes lays 8 frames out as a 4x2 GRID rather
	// than one row, so a naive single-row slice stacks two frames per tile. When
	// the count disagrees, re-derive the frames straight from the alpha: take the
	// full opaque bounding box, infer the grid shape (cols x rows = expected) whose
	// aspect best matches the box, and slice it row-major. Band-agnostic, bounded
	// (no runaway tile size), and fires only when detection already disagrees, so
	// cleanly detected sheets are untouched.
	if expected > 0 && len(cells) != expected {
		minX, minY, maxX, maxY := w, h, 0, 0
		found := false
		for y := 0; y < h; y++ {
			base := y * w
			for x := 0; x < w; x++ {
				if alpha[base+x] > 0 {
					found = true
					if x < minX {
						minX = x
					}
					if x > maxX {
						maxX = x
					}
					if y < minY {
						minY = y
					}
					if y > maxY {
						maxY = y
					}
				}
			}
		}
		if found {
			bw, bh := maxX+1-minX, maxY+1-minY
			// Pick cols x rows = expected whose aspect (cols/rows) best matches the
			// box aspect (bw/bh). Compared by cross-multiplication so it stays
			// integer: bw/bh == cols/rows  <=>  bw*rows == cols*bh.
			cols, rows, bestErr := expected, 1, -1
			for c := 1; c <= expected; c++ {
				if expected%c != 0 {
					continue
				}
				r := expected / c
				e := bw*r - c*bh
				if e < 0 {
					e = -e
				}
				if bestErr < 0 || e < bestErr {
					bestErr, cols, rows = e, c, r
				}
			}
			cells = cells[:0]
			for r := 0; r < rows; r++ {
				ys := minY + bh*r/rows
				ye := minY + bh*(r+1)/rows
				for c := 0; c < cols; c++ {
					xs := minX + bw*c/cols
					xe := minX + bw*(c+1)/cols
					cells = append(cells, cell{band: 0, col: span{xs, xe}, y: span{ys, ye}})
				}
			}
		}
	}

	frames := make([]image.Rectangle, 0, len(cells))
	for _, c := range cells {
		frames = append(frames, image.Rect(c.col.start, c.y.start, c.col.end, c.y.end))
	}
	return frames
}

type span struct{ start, end int }

// runs returns contiguous true-spans of mask whose length exceeds minLen.
func runs(mask []bool, minLen int) []span {
	if minLen < 1 {
		minLen = 1
	}
	var out []span
	start := -1
	for i, v := range mask {
		if v && start < 0 {
			start = i
		} else if !v && start >= 0 {
			if i-start > minLen {
				out = append(out, span{start, i})
			}
			start = -1
		}
	}
	if start >= 0 && len(mask)-start > minLen {
		out = append(out, span{start, len(mask)})
	}
	return out
}

// reflowToGrid tight-crops each cell to its opaque bounds and composes the
// crops into a columns x rows grid of equal square tiles with transparent
// padding, each sprite centered in its tile. Unlike reflowToRow, empty cells
// are kept (as fully transparent tiles) so grid positions stay stable.
func reflowToGrid(img *image.RGBA, alpha []uint8, cells []image.Rectangle, columns, rows int) *image.NRGBA {
	w := img.Bounds().Dx()
	crops := make([]image.Rectangle, len(cells))
	maxW, maxH := 0, 0
	for i, f := range cells {
		minX, minY, maxX, maxY := f.Max.X, f.Max.Y, f.Min.X, f.Min.Y
		found := false
		for y := f.Min.Y; y < f.Max.Y; y++ {
			for x := f.Min.X; x < f.Max.X; x++ {
				if alpha[y*w+x] > 0 {
					found = true
					if x < minX {
						minX = x
					}
					if x > maxX {
						maxX = x
					}
					if y < minY {
						minY = y
					}
					if y > maxY {
						maxY = y
					}
				}
			}
		}
		if !found {
			continue // crops[i] stays empty
		}
		r := image.Rect(minX, minY, maxX+1, maxY+1)
		crops[i] = r
		if r.Dx() > maxW {
			maxW = r.Dx()
		}
		if r.Dy() > maxH {
			maxH = r.Dy()
		}
	}
	if maxW == 0 && maxH == 0 {
		return image.NewNRGBA(image.Rect(0, 0, 1, 1))
	}

	cell := maxW
	if maxH > cell {
		cell = maxH
	}
	pad := cell * 12 / 100
	tile := cell + pad*2
	out := image.NewNRGBA(image.Rect(0, 0, tile*columns, tile*rows))

	for i, c := range crops {
		if c.Empty() {
			continue
		}
		cw, ch := c.Dx(), c.Dy()
		ox := (i%columns)*tile + (tile-cw)/2
		oy := (i/columns)*tile + (tile-ch)/2
		for y := 0; y < ch; y++ {
			for x := 0; x < cw; x++ {
				a := alpha[(c.Min.Y+y)*w+(c.Min.X+x)]
				if a == 0 {
					continue
				}
				so := img.PixOffset(c.Min.X+x, c.Min.Y+y)
				do := out.PixOffset(ox+x, oy+y)
				out.Pix[do] = img.Pix[so]
				out.Pix[do+1] = img.Pix[so+1]
				out.Pix[do+2] = img.Pix[so+2]
				out.Pix[do+3] = a
			}
		}
	}
	return out
}

// reflowToRow tight-crops each detected frame to its opaque bounds and composes
// them into a single horizontal row of equal square tiles with transparent
// padding, each sprite centered in its tile.
func reflowToRow(img *image.RGBA, alpha []uint8, frames []image.Rectangle) *image.NRGBA {
	w := img.Bounds().Dx()
	type crop struct {
		r image.Rectangle // tight opaque bounds in source space
	}
	crops := make([]crop, 0, len(frames))
	maxW, maxH := 0, 0
	for _, f := range frames {
		minX, minY, maxX, maxY := f.Max.X, f.Max.Y, f.Min.X, f.Min.Y
		found := false
		for y := f.Min.Y; y < f.Max.Y; y++ {
			for x := f.Min.X; x < f.Max.X; x++ {
				if alpha[y*w+x] > 0 {
					found = true
					if x < minX {
						minX = x
					}
					if x > maxX {
						maxX = x
					}
					if y < minY {
						minY = y
					}
					if y > maxY {
						maxY = y
					}
				}
			}
		}
		if !found {
			continue
		}
		r := image.Rect(minX, minY, maxX+1, maxY+1)
		crops = append(crops, crop{r})
		if r.Dx() > maxW {
			maxW = r.Dx()
		}
		if r.Dy() > maxH {
			maxH = r.Dy()
		}
	}
	if len(crops) == 0 {
		return image.NewNRGBA(image.Rect(0, 0, 1, 1))
	}

	cell := maxW
	if maxH > cell {
		cell = maxH
	}
	pad := cell * 12 / 100
	tile := cell + pad*2
	n := len(crops)
	out := image.NewNRGBA(image.Rect(0, 0, tile*n, tile))

	for i, c := range crops {
		cw, ch := c.r.Dx(), c.r.Dy()
		ox := i*tile + (tile-cw)/2
		oy := (tile - ch) / 2
		for y := 0; y < ch; y++ {
			for x := 0; x < cw; x++ {
				a := alpha[(c.r.Min.Y+y)*w+(c.r.Min.X+x)]
				if a == 0 {
					continue
				}
				so := img.PixOffset(c.r.Min.X+x, c.r.Min.Y+y)
				do := out.PixOffset(ox+x, oy+y)
				out.Pix[do] = img.Pix[so]
				out.Pix[do+1] = img.Pix[so+1]
				out.Pix[do+2] = img.Pix[so+2]
				out.Pix[do+3] = a
			}
		}
	}
	return out
}
