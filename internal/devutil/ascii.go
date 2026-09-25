// Package devutil contains debugging helpers used by tests and dev tools.
package devutil

import (
	"fmt"
	"image"
	"strings"
)

// ASCII renders a luminance-difference preview of img (relative to the
// top-left background pixel) using cols characters per row. It is useful to
// sanity check layouts without an image viewer.
func ASCII(img *image.RGBA, cols int) string {
	ramp := []byte(" .:-=+*#%@")
	b := img.Bounds()
	if cols <= 0 {
		cols = 120
	}
	cw := float64(b.Dx()) / float64(cols)
	ch := cw * 2
	rows := int(float64(b.Dy()) / ch)
	bg := img.RGBAAt(0, 0)
	var sb strings.Builder
	fmt.Fprintf(&sb, "%dx%d px\n", b.Dx(), b.Dy())
	for r := 0; r < rows; r++ {
		for c := 0; c < cols; c++ {
			x0, y0 := int(float64(c)*cw), int(float64(r)*ch)
			x1, y1 := int(float64(c+1)*cw), int(float64(r+1)*ch)
			var sum, n float64
			for y := y0; y < y1; y++ {
				for x := x0; x < x1; x++ {
					p := img.RGBAAt(x, y)
					d := (abs(int(p.R)-int(bg.R)) + abs(int(p.G)-int(bg.G)) + abs(int(p.B)-int(bg.B))) / 3
					sum += float64(d)
					n++
				}
			}
			v := 0.0
			if n > 0 {
				v = sum / n / 255
			}
			i := int(v * 2.5 * float64(len(ramp)-1))
			if i >= len(ramp) {
				i = len(ramp) - 1
			}
			sb.WriteByte(ramp[i])
		}
		sb.WriteByte('\n')
	}
	return sb.String()
}

func abs(v int) int {
	if v < 0 {
		return -v
	}
	return v
}
