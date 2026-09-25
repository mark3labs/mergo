package tui

import (
	"image"
	"image/color"
	"math"

	uv "github.com/charmbracelet/ultraviolet"
)

// Half-block rendering: every terminal cell shows two vertically stacked
// "pixels" using the upper half block glyph (▀) with the foreground color
// as the top pixel and the background color as the bottom pixel.

const upperHalf = "▀"

var (
	srgbToLinear [256]float32
	linearToSRGB [4096]uint8
)

func init() {
	for i := range srgbToLinear {
		v := float64(i) / 255
		if v <= 0.04045 {
			v /= 12.92
		} else {
			v = math.Pow((v+0.055)/1.055, 2.4)
		}
		srgbToLinear[i] = float32(v)
	}
	for i := range linearToSRGB {
		v := float64(i) / float64(len(linearToSRGB)-1)
		if v <= 0.0031308 {
			v *= 12.92
		} else {
			v = 1.055*math.Pow(v, 1/2.4) - 0.055
		}
		linearToSRGB[i] = uint8(math.Round(v * 255))
	}
}

func toSRGB(v float32) uint8 {
	i := int(v*float32(len(linearToSRGB)-1) + 0.5)
	if i < 0 {
		i = 0
	}
	if i >= len(linearToSRGB) {
		i = len(linearToSRGB) - 1
	}
	return linearToSRGB[i]
}

// downsample resizes img to w x h using gamma-correct area averaging. The
// alpha channel is ignored (images are expected to be opaque).
func downsample(img *image.RGBA, w, h int) *image.RGBA {
	out := image.NewRGBA(image.Rect(0, 0, w, h))
	b := img.Bounds()
	sw, sh := b.Dx(), b.Dy()
	if w <= 0 || h <= 0 || sw == 0 || sh == 0 {
		return out
	}
	fx := float64(sw) / float64(w)
	fy := float64(sh) / float64(h)
	for y := 0; y < h; y++ {
		y0 := int(float64(y) * fy)
		y1 := max(int(float64(y+1)*fy), y0+1)
		y1 = min(y1, sh)
		for x := 0; x < w; x++ {
			x0 := int(float64(x) * fx)
			x1 := max(int(float64(x+1)*fx), x0+1)
			x1 = min(x1, sw)
			var r, g, bl float32
			n := 0
			for yy := y0; yy < y1; yy++ {
				row := img.Pix[(yy+b.Min.Y-img.Rect.Min.Y)*img.Stride:]
				for xx := x0; xx < x1; xx++ {
					i := (xx + b.Min.X - img.Rect.Min.X) * 4
					r += srgbToLinear[row[i]]
					g += srgbToLinear[row[i+1]]
					bl += srgbToLinear[row[i+2]]
					n++
				}
			}
			o := out.PixOffset(x, y)
			if n == 0 {
				continue
			}
			inv := 1 / float32(n)
			out.Pix[o] = toSRGB(r * inv)
			out.Pix[o+1] = toSRGB(g * inv)
			out.Pix[o+2] = toSRGB(bl * inv)
			out.Pix[o+3] = 255
		}
	}
	return out
}

// halfBlocks converts img into a cols x rows grid of half block cells and
// returns the rendered, styled lines. img is resampled to cols x rows*2.
func halfBlocks(img *image.RGBA, cols, rows int) []string {
	if cols <= 0 || rows <= 0 {
		return nil
	}
	px := img
	if img.Bounds().Dx() != cols || img.Bounds().Dy() != rows*2 {
		px = downsample(img, cols, rows*2)
	}
	buf := uv.NewBuffer(cols, rows)
	for y := 0; y < rows; y++ {
		for x := 0; x < cols; x++ {
			top := px.RGBAAt(x, 2*y)
			bot := px.RGBAAt(x, 2*y+1)
			top.A, bot.A = 255, 255
			var c uv.Cell
			if top == bot {
				c = uv.Cell{Content: " ", Width: 1, Style: uv.Style{Bg: color.RGBA(bot)}}
			} else {
				c = uv.Cell{Content: upperHalf, Width: 1, Style: uv.Style{Fg: color.RGBA(top), Bg: color.RGBA(bot)}}
			}
			buf.SetCell(x, y, &c)
		}
	}
	lines := make([]string, rows)
	for y := 0; y < rows; y++ {
		lines[y] = buf.Line(y).Render()
	}
	return lines
}
