// Package mermaid is the public entry point of the rendering engine. Diagram
// implementations register themselves via blank imports in register_*.go
// files in this package.
package mermaid

import (
	"bytes"
	"image"
	"image/png"

	"github.com/mark3labs/mergo/internal/diagram"
	"github.com/mark3labs/mergo/internal/scene"
	"github.com/mark3labs/mergo/internal/theme"
)

// Render parses Mermaid source and returns a vector scene.
func Render(src string, th *theme.Theme) (*scene.Scene, error) {
	if th == nil {
		th = theme.Default()
	}
	return diagram.Render(src, th)
}

// RenderImage renders Mermaid source into a raster image at the given scale.
func RenderImage(src string, th *theme.Theme, scale float64) (*image.RGBA, error) {
	sc, err := Render(src, th)
	if err != nil {
		return nil, err
	}
	return sc.Render(scene.RenderOptions{Scale: scale}), nil
}

// RenderPNG renders Mermaid source into PNG bytes.
func RenderPNG(src string, th *theme.Theme, scale float64) ([]byte, error) {
	img, err := RenderImage(src, th, scale)
	if err != nil {
		return nil, err
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// Types lists supported diagram types.
func Types() []string { return diagram.Types() }
