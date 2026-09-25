package scene

import (
	"fmt"
	"math"
	"os"
	"strings"
	"sync"

	"golang.org/x/image/font"
	"golang.org/x/image/font/gofont/gobold"
	"golang.org/x/image/font/gofont/gobolditalic"
	"golang.org/x/image/font/gofont/goitalic"
	"golang.org/x/image/font/gofont/gomono"
	"golang.org/x/image/font/gofont/gomonobold"
	"golang.org/x/image/font/gofont/goregular"
	"golang.org/x/image/font/opentype"
	"golang.org/x/image/math/fixed"
)

type fontVariant int

const (
	varRegular fontVariant = iota
	varBold
	varItalic
	varBoldItalic
	varMono
	varMonoBold
	numVariants
)

var (
	fontMu    sync.Mutex
	fonts     [numVariants]*opentype.Font
	faceCache = map[faceKey]font.Face{}
)

type faceKey struct {
	v    fontVariant
	size int // size * 16
}

func init() {
	srcs := [numVariants][]byte{
		goregular.TTF, gobold.TTF, goitalic.TTF, gobolditalic.TTF, gomono.TTF, gomonobold.TTF,
	}
	for i, b := range srcs {
		f, err := opentype.Parse(b)
		if err != nil {
			panic(err)
		}
		fonts[i] = f
	}
}

// LoadFonts replaces the proportional font family with TrueType/OpenType files.
// Any empty path keeps the current font for that variant. If only a regular
// font is given, it is also used for the other proportional variants.
func LoadFonts(regular, bold string) error {
	fontMu.Lock()
	defer fontMu.Unlock()
	load := func(p string) (*opentype.Font, error) {
		b, err := os.ReadFile(p)
		if err != nil {
			return nil, err
		}
		f, err := opentype.Parse(b)
		if err != nil {
			return nil, fmt.Errorf("parse font %s: %w", p, err)
		}
		return f, nil
	}
	if regular != "" {
		f, err := load(regular)
		if err != nil {
			return err
		}
		fonts[varRegular] = f
		fonts[varItalic] = f
		if bold == "" {
			fonts[varBold] = f
			fonts[varBoldItalic] = f
		}
	}
	if bold != "" {
		f, err := load(bold)
		if err != nil {
			return err
		}
		fonts[varBold] = f
		fonts[varBoldItalic] = f
	}
	faceCache = map[faceKey]font.Face{}
	return nil
}

func variantOf(f Font) fontVariant {
	switch {
	case f.Mono && f.Bold:
		return varMonoBold
	case f.Mono:
		return varMono
	case f.Bold && f.Italic:
		return varBoldItalic
	case f.Bold:
		return varBold
	case f.Italic:
		return varItalic
	}
	return varRegular
}

// face returns a cached face for the font at the given pixel size.
func face(f Font, px float64) font.Face {
	if px < 1 {
		px = 1
	}
	k := faceKey{variantOf(f), int(math.Round(px * 16))}
	fontMu.Lock()
	defer fontMu.Unlock()
	if fc, ok := faceCache[k]; ok {
		return fc
	}
	fc, err := opentype.NewFace(fonts[k.v], &opentype.FaceOptions{
		Size:    float64(k.size) / 16,
		DPI:     72,
		Hinting: font.HintingNone,
	})
	if err != nil {
		panic(err)
	}
	if len(faceCache) > 256 {
		faceCache = map[faceKey]font.Face{}
	}
	faceCache[k] = fc
	return fc
}

const measureSize = 64.0

// MeasureText returns the advance width of a single line of text (inline
// style markers select bold/italic faces for their spans).
func MeasureText(s string, f Font) float64 {
	if s == "" || f.Size <= 0 {
		return 0
	}
	if HasStyle(s) {
		var w float64
		for _, r := range runs(s, f) {
			w += MeasureText(r.s, r.f)
		}
		return w
	}
	fc := face(f, measureSize)
	fontMu.Lock()
	adv := font.MeasureString(fc, s)
	fontMu.Unlock()
	return fix2f(adv) * f.Size / measureSize
}

// Ascent returns the font ascent in pixels.
func Ascent(f Font) float64 {
	fc := face(f, measureSize)
	fontMu.Lock()
	m := fc.Metrics()
	fontMu.Unlock()
	return fix2f(m.Ascent) * f.Size / measureSize
}

// Descent returns the font descent in pixels.
func Descent(f Font) float64 {
	fc := face(f, measureSize)
	fontMu.Lock()
	m := fc.Metrics()
	fontMu.Unlock()
	return fix2f(m.Descent) * f.Size / measureSize
}

// MeasureBlock measures a multi-line ('\n' separated) text block.
// lineHeight is a multiplier of the font size (0 means 1.25).
func MeasureBlock(s string, f Font, lineHeight float64) (w, h float64) {
	if lineHeight == 0 {
		lineHeight = 1.25
	}
	lines := strings.Split(s, "\n")
	for _, l := range lines {
		w = math.Max(w, MeasureText(l, f))
	}
	h = float64(len(lines)) * f.Size * lineHeight
	return w, h
}

// WrapText word-wraps s so that no line is wider than maxW (existing
// newlines are preserved).
func WrapText(s string, f Font, maxW float64) string {
	if maxW <= 0 {
		return s
	}
	var out []string
	for para := range strings.SplitSeq(s, "\n") {
		words := strings.Fields(para)
		if len(words) == 0 {
			out = append(out, "")
			continue
		}
		line := words[0]
		// style spans may cross wrap points: close them at the end of a
		// line and re-open them on the next one
		var st styleState
		for _, r := range line {
			st.apply(r)
		}
		for _, w := range words[1:] {
			cand := line + " " + w
			if MeasureText(cand, f) > maxW {
				out = append(out, line+st.closers())
				line = st.openers() + w
			} else {
				line = cand
			}
			for _, r := range w {
				st.apply(r)
			}
		}
		out = append(out, line)
	}
	return strings.Join(out, "\n")
}

func fix2f(v fixed.Int26_6) float64 { return float64(v) / 64 }
