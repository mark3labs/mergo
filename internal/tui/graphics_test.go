package tui

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"testing"

	"github.com/mark3labs/mergo/internal/scene"
)

func TestGraphicsCapability(t *testing.T) {
	g := NewGraphics()
	g.SetCapability(CapKitty)
	if g.capability != RendererKitty {
		t.Errorf("SetCapability failed: got %v, want RendererKitty", g.capability)
	}
}

func TestHalfBlockDownsample(t *testing.T) {
	// Create a simple test image: 8x8 red pixels
	img := image.NewRGBA(image.Rect(0, 0, 8, 8))
	for y := range 8 {
		for x := range 8 {
			img.SetRGBA(x, y, color.RGBA{255, 0, 0, 255})
		}
	}

	// Downsample to 4x4
	result := downsampleImage(img, 4, 4)

	if result.Bounds().Dx() != 4 || result.Bounds().Dy() != 4 {
		t.Errorf("Downsample dimensions: got %dx%d, want 4x4",
			result.Bounds().Dx(), result.Bounds().Dy())
	}

	// All pixels should still be red (or close)
	c := result.At(0, 0).(color.RGBA)
	if c.R < 250 || c.G > 5 || c.B > 5 {
		t.Errorf("Downsample color: got %+v, want red", c)
	}
}

func TestFormatRGB(t *testing.T) {
	tests := []struct {
		c    color.RGBA
		want string
	}{
		{color.RGBA{255, 0, 0, 255}, "\x1b[38;2;255;0;0"},
		{color.RGBA{0, 255, 0, 255}, "\x1b[38;2;0;255;0"},
		{color.RGBA{0, 0, 255, 255}, "\x1b[38;2;0;0;255"},
	}

	for _, tt := range tests {
		got := formatRGB(tt.c)
		if got != tt.want {
			t.Errorf("formatRGB(%+v): got %q, want %q", tt.c, got, tt.want)
		}
	}
}

func TestBase64EncodeChunk(t *testing.T) {
	tests := []struct {
		name string
		data []byte
		// We'll just verify it doesn't crash and produces something
		shouldNotEmpty bool
	}{
		{"empty", []byte{}, false},
		{"single byte", []byte{0}, true},
		{"three bytes", []byte{0, 1, 2}, true},
		{"long", make([]byte, 1000), true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := base64EncodeChunk(tt.data)
			if tt.shouldNotEmpty && result == "" {
				t.Error("Expected non-empty result")
			}
		})
	}
}

// CreateTestPNG creates a simple test PNG for testing.
func CreateTestPNG(width, height int) []byte {
	img := image.NewRGBA(image.Rect(0, 0, width, height))
	for y := range height {
		for x := range width {
			img.SetRGBA(x, y, color.RGBA{
				R: uint8((x * 255) / width),
				G: uint8((y * 255) / height),
				B: 128,
				A: 255,
			})
		}
	}

	var buf bytes.Buffer
	png.Encode(&buf, img)
	return buf.Bytes()
}

func TestRenderToPNG(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 10, 10))
	result := scene.RenderToPNG(img)
	if len(result) == 0 {
		t.Error("RenderToPNG returned empty result")
	}

	// Verify it's valid PNG by decoding
	decodedImg, err := decodePNG(result)
	if err != nil {
		t.Errorf("Decoded PNG failed: %v", err)
	}
	if decodedImg.Bounds().Dx() != 10 || decodedImg.Bounds().Dy() != 10 {
		t.Errorf("Decoded image dimensions: got %dx%d, want 10x10",
			decodedImg.Bounds().Dx(), decodedImg.Bounds().Dy())
	}
}
