package tui

import (
	"fmt"
	"image"
	"math"
	"slices"
	"strconv"
	"strings"
)

// Sixel graphics.
//
// Sixel images are drawn at the cursor position and become part of the
// terminal's cell contents: text written over them replaces the pixels of
// those cells. The viewer therefore keeps the image area as a grid of
// default blank cells (which the cell renderer never rewrites while they
// stay unchanged) and paints the image over them. Overlays (help, toasts,
// error panels, the theme picker) are left out of the image as
// transparent pixels (P2=1), so they stay visible; whenever the overlays
// change, the image is painted again.
//
// See https://vt100.net/docs/vt3xx-gp/chapter14.html

// sixelMaxColors is the palette size. 256 registers are the de facto
// standard (xterm with -ti vt340, foot, WezTerm, mlterm, …); one index is
// reserved for transparent pixels.
const sixelMaxColors = 255

// sixelTransparent marks pixels that are not painted.
const sixelTransparent = 255

// encodeSixel encodes img as a complete sixel DCS sequence. Pixels inside
// the mask rectangles (in image coordinates) are left transparent.
func encodeSixel(img *image.RGBA, mask []image.Rectangle) string {
	b := img.Bounds()
	w, h := b.Dx(), b.Dy()
	if w <= 0 || h <= 0 {
		return ""
	}
	pal, idx := quantize(img, mask)

	var sb strings.Builder
	sb.Grow(w*h/4 + len(pal)*20 + 64)
	// P1=0 (aspect from the raster attributes), P2=1 (unset pixels stay
	// transparent), then 1:1 pixel aspect and the image size.
	fmt.Fprintf(&sb, "\x1bP0;1;0q\"1;1;%d;%d", w, h)
	for i, c := range pal {
		fmt.Fprintf(&sb, "#%d;2;%d;%d;%d", i, pct(c[0]), pct(c[1]), pct(c[2]))
	}

	// One bit row (6 pixels high) per palette color and band.
	rows := make([][]byte, len(pal))
	inBand := make([]bool, len(pal))
	var used []int
	for y0 := 0; y0 < h; y0 += 6 {
		used = used[:0]
		for dy := 0; dy < 6 && y0+dy < h; dy++ {
			line := idx[(y0+dy)*w : (y0+dy+1)*w]
			bit := byte(1) << dy
			for x, c := range line {
				if c == sixelTransparent {
					continue
				}
				r := rows[c]
				if r == nil {
					r = make([]byte, w)
					rows[c] = r
				}
				if !inBand[c] {
					inBand[c] = true
					used = append(used, int(c))
				}
				r[x] |= bit
			}
		}
		for i, c := range used {
			if i > 0 {
				sb.WriteByte('$') // back to the start of the band
			}
			sb.WriteByte('#')
			sb.WriteString(strconv.Itoa(c))
			writeSixelRow(&sb, rows[c])
			clear(rows[c])
			inBand[c] = false
		}
		sb.WriteByte('-') // next band
	}
	sb.WriteString("\x1b\\")
	return sb.String()
}

// writeSixelRow writes one color's bits of a band, run-length encoded and
// without trailing empty columns.
func writeSixelRow(sb *strings.Builder, row []byte) {
	end := len(row)
	for end > 0 && row[end-1] == 0 {
		end--
	}
	for x := 0; x < end; {
		v := row[x]
		n := 1
		for x+n < end && row[x+n] == v {
			n++
		}
		ch := v + '?'
		if n > 3 {
			sb.WriteByte('!')
			sb.WriteString(strconv.Itoa(n))
			sb.WriteByte(ch)
		} else {
			for range n {
				sb.WriteByte(ch)
			}
		}
		x += n
	}
}

// pct converts an 8-bit channel to the 0..100 range of sixel colors.
func pct(v uint8) int { return (int(v)*100 + 127) / 255 }

// colorBin accumulates the pixels falling into one 15-bit color bin.
type colorBin struct {
	n       uint32
	r, g, b uint64
}

func (c colorBin) avg() [3]uint8 {
	return [3]uint8{uint8(c.r / uint64(c.n)), uint8(c.g / uint64(c.n)), uint8(c.b / uint64(c.n))}
}

// quantize builds a palette of at most sixelMaxColors colors for img and
// returns it with the palette index of every pixel (row-major, masked
// pixels set to sixelTransparent). Colors are binned at 5 bits per
// channel; if there are more bins than palette entries, the bins are
// reduced with a weighted median cut.
func quantize(img *image.RGBA, mask []image.Rectangle) ([][3]uint8, []uint8) {
	b := img.Bounds()
	w, h := b.Dx(), b.Dy()
	masked := func(x, y int) bool {
		for _, r := range mask {
			if x >= r.Min.X && x < r.Max.X && y >= r.Min.Y && y < r.Max.Y {
				return true
			}
		}
		return false
	}

	idx := make([]uint8, w*h)
	keys := make([]uint16, w*h)
	bins := make([]colorBin, 1<<15)
	for y := range h {
		row := img.Pix[(y+b.Min.Y-img.Rect.Min.Y)*img.Stride:]
		for x := range w {
			if len(mask) > 0 && masked(x, y) {
				idx[y*w+x] = sixelTransparent
				continue
			}
			i := (x + b.Min.X - img.Rect.Min.X) * 4
			r, g, bl := row[i], row[i+1], row[i+2]
			k := uint16(r>>3)<<10 | uint16(g>>3)<<5 | uint16(bl>>3)
			keys[y*w+x] = k
			c := &bins[k]
			c.n++
			c.r += uint64(r)
			c.g += uint64(g)
			c.b += uint64(bl)
		}
	}

	var usedBins []int
	for k := range bins {
		if bins[k].n > 0 {
			usedBins = append(usedBins, k)
		}
	}
	lut := make([]uint8, len(bins))
	var pal [][3]uint8
	if len(usedBins) <= sixelMaxColors {
		for i, k := range usedBins {
			lut[k] = uint8(i)
			pal = append(pal, bins[k].avg())
		}
	} else {
		pal = medianCut(bins, usedBins, sixelMaxColors)
		for _, k := range usedBins {
			lut[k] = nearest(pal, bins[k].avg())
		}
	}
	for i, c := range idx {
		if c != sixelTransparent {
			idx[i] = lut[keys[i]]
		}
	}
	return pal, idx
}

// medianCut reduces the used bins to n colors.
func medianCut(bins []colorBin, used []int, n int) [][3]uint8 {
	type box struct {
		keys []int
		n    uint64
	}
	channel := func(k, ch int) uint8 { return bins[k].avg()[ch] }
	spread := func(bx box) (int, int) {
		best, bestCh := -1, 0
		for ch := range 3 {
			lo, hi := 255, 0
			for _, k := range bx.keys {
				v := int(channel(k, ch))
				lo, hi = min(lo, v), max(hi, v)
			}
			if hi-lo > best {
				best, bestCh = hi-lo, ch
			}
		}
		return best, bestCh
	}
	total := func(keys []int) uint64 {
		var s uint64
		for _, k := range keys {
			s += uint64(bins[k].n)
		}
		return s
	}

	boxes := []box{{keys: slices.Clone(used), n: total(used)}}
	for len(boxes) < n {
		// split the box with the largest weighted spread
		bi, bestScore, bestCh := -1, 0.0, 0
		for i, bx := range boxes {
			if len(bx.keys) < 2 {
				continue
			}
			s, ch := spread(bx)
			// sqrt weighting keeps rare anti-aliasing shades from being
			// starved while still favoring large boxes
			score := float64(s) * math.Sqrt(float64(bx.n))
			if score > bestScore {
				bi, bestScore, bestCh = i, score, ch
			}
		}
		if bi < 0 {
			break
		}
		bx := boxes[bi]
		slices.SortFunc(bx.keys, func(a, b int) int { return int(channel(a, bestCh)) - int(channel(b, bestCh)) })
		half, acc, cut := bx.n/2, uint64(0), 1
		for i, k := range bx.keys {
			acc += uint64(bins[k].n)
			if acc >= half {
				cut = min(max(i+1, 1), len(bx.keys)-1)
				break
			}
		}
		lo, hi := bx.keys[:cut], bx.keys[cut:]
		boxes[bi] = box{keys: lo, n: total(lo)}
		boxes = append(boxes, box{keys: hi, n: total(hi)})
	}

	pal := make([][3]uint8, 0, len(boxes))
	for _, bx := range boxes {
		var c colorBin
		for _, k := range bx.keys {
			c.n += bins[k].n
			c.r += bins[k].r
			c.g += bins[k].g
			c.b += bins[k].b
		}
		pal = append(pal, c.avg())
	}
	return pal
}

// nearest returns the index of the palette color closest to c.
func nearest(pal [][3]uint8, c [3]uint8) uint8 {
	best, bestD := 0, 1<<30
	for i, p := range pal {
		dr := int(p[0]) - int(c[0])
		dg := int(p[1]) - int(c[1])
		db := int(p[2]) - int(c[2])
		// weighted for perceived brightness
		d := 3*dr*dr + 4*dg*dg + 2*db*db
		if d < bestD {
			best, bestD = i, d
			if d == 0 {
				break
			}
		}
	}
	return uint8(best)
}

// sixelAt paints a sixel sequence with its top-left corner at the 0-based
// cell (row, col). The cursor (and pen) are saved and restored so the TUI
// renderer's idea of the cursor is not disturbed.
func sixelAt(row, col int, data string) string {
	return "\x1b7" + fmt.Sprintf("\x1b[%d;%dH", row+1, col+1) + data + "\x1b8"
}
