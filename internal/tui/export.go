package tui

import (
	"image"
	"image/png"
	"os"
)

// writePNG writes an image.Image as PNG to a file.
func writePNG(path string, img image.Image) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	return png.Encode(f, img)
}
