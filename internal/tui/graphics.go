package tui

import (
	"bytes"
	"fmt"
	"image"
	"image/color"
	"image/png"

	"github.com/charmbracelet/x/ansi/kitty"
)

// GraphicsCapability represents terminal graphics support.
type GraphicsCapability int

const (
	CapNone GraphicsCapability = iota
	CapKitty
	CapHalfBlock
	CapAuto
)

// RendererType is the type of renderer in use.
type RendererType int

const (
	RendererAuto RendererType = iota
	RendererKitty
	RendererHalfBlock
)

// Graphics handles rendering to the terminal.
type Graphics struct {
	capability RendererType
	cellSize   CellSize
	image      []byte // Current PNG image
	imageID    int    // Current image ID for kitty (0 or 1 for double-buffering)

	// Kitty state
	placementID int
	placedCols  int
	placedRows  int

	// Half-block state
	lastPNG []byte
}

// NewGraphics creates a new graphics renderer.
func NewGraphics() *Graphics {
	return &Graphics{
		capability: RendererAuto,
		cellSize:   CellSize{Width: 8, Height: 16},
	}
}

// SetCapability sets the detected graphics capability.
func (g *Graphics) SetCapability(cap GraphicsCapability) {
	switch cap {
	case CapKitty:
		g.capability = RendererKitty
	case CapHalfBlock:
		g.capability = RendererHalfBlock
	default:
		g.capability = RendererHalfBlock // Fallback to half-block
	}
}

// UpdateCellSize updates the terminal cell size.
func (g *Graphics) UpdateCellSize(size CellSize) {
	g.cellSize = size
}

// SetImage sets the PNG image to render.
func (g *Graphics) SetImage(pngData []byte) {
	g.image = pngData
}

// ToggleRenderer cycles through available renderers.
func (g *Graphics) ToggleRenderer() {
	if g.capability == RendererKitty {
		g.capability = RendererHalfBlock
	} else if g.capability == RendererHalfBlock && true { // Check if auto was kitty
		g.capability = RendererKitty
	} else {
		g.capability = RendererHalfBlock
	}
}

// Render returns the rendered view content.
func (g *Graphics) Render(pngData []byte) string {
	if pngData == nil {
		return ""
	}

	g.image = pngData

	switch g.capability {
	case RendererKitty:
		return g.renderKitty(pngData)
	case RendererHalfBlock:
		return g.renderHalfBlock(pngData)
	default:
		return g.renderHalfBlock(pngData)
	}
}

// renderKitty renders using the Kitty graphics protocol.
func (g *Graphics) renderKitty(pngData []byte) string {
	// Decode PNG to get dimensions
	img, err := decodePNG(pngData)
	if err != nil {
		return fmt.Sprintf("Error decoding image: %v", err)
	}

	bounds := img.Bounds()
	imgWidth := bounds.Dx()
	imgHeight := bounds.Dy()

	// Calculate placement dimensions
	cols := max(1, imgWidth/g.cellSize.Width)
	rows := max(1, imgHeight/g.cellSize.Height)

	// Toggle image ID for double-buffering
	if g.imageID == 0 {
		g.imageID = 1
	} else {
		// TODO: Delete previous image
		g.imageID = 0
	}

	// Send image with kitty protocol
	opts := kitty.Options{
		Action:           kitty.Transmit,
		Format:           kitty.PNG,
		ID:               g.imageID,
		Columns:          cols,
		Rows:             rows,
		Chunk:            true,
		VirtualPlacement: true,
	}

	// Build escape sequence
	var buf bytes.Buffer
	buf.WriteString("\x1b_G")

	// Add options
	for _, opt := range opts.Options() {
		buf.WriteString(opt)
		buf.WriteString(",")
	}

	// Add PNG data in chunks
	const chunkSize = 4096
	for i := 0; i < len(pngData); i += chunkSize {
		end := min(i+chunkSize, len(pngData))
		chunk := pngData[i:end]

		// Base64 encode chunk
		encoded := base64EncodeChunk(chunk)
		if i+chunkSize < len(pngData) {
			buf.WriteString(fmt.Sprintf("m=1;%s", encoded))
		} else {
			buf.WriteString(fmt.Sprintf("m=0;%s", encoded))
		}

		if i+chunkSize < len(pngData) {
			buf.WriteString("\x1b\\\x1b_G")
		}
	}

	buf.WriteString("\x1b\\")

	// Place the image using Unicode placeholders
	buf.WriteString(kittyPlaceholders(cols, rows, g.imageID))

	return buf.String()
}

// renderHalfBlock renders using half-block characters with truecolor.
func (g *Graphics) renderHalfBlock(pngData []byte) string {
	// Decode PNG
	img, err := decodePNG(pngData)
	if err != nil {
		return fmt.Sprintf("Error: %v", err)
	}

	bounds := img.Bounds()
	imgWidth := bounds.Dx()
	imgHeight := bounds.Dy()

	// Scale to terminal dimensions
	// Each half-block cell represents cellWidth x (2 * cellHeight) pixels
	termCols := max(1, imgWidth/g.cellSize.Width)
	termRows := max(1, imgHeight/(2*g.cellSize.Height))

	// Downsample image to terminal size
	downsampled := downsampleImage(img, termCols, termRows*2)

	// Generate half-block cells
	var buf bytes.Buffer
	for y := range termRows {
		for x := range termCols {
			top := downsampled.At(x, y*2).(color.RGBA)
			bottom := downsampled.At(x, y*2+1).(color.RGBA)

			// Write half-block with colors
			fgStr := formatRGB(top)
			bgStr := formatRGB(bottom)
			buf.WriteString(fmt.Sprintf("%s;%s;1m▀", fgStr, bgStr))
		}
		buf.WriteString("\r\n")
	}

	return buf.String()
}

// Helper functions

func decodePNG(data []byte) (image.Image, error) {
	return png.Decode(bytes.NewReader(data))
}

func downsampleImage(img image.Image, targetW, targetH int) *image.RGBA {
	bounds := img.Bounds()
	srcW := bounds.Dx()
	srcH := bounds.Dy()

	// Create output image
	out := image.NewRGBA(image.Rect(0, 0, targetW, targetH))

	// Area-average downsampling
	scaleX := float64(srcW) / float64(targetW)
	scaleY := float64(srcH) / float64(targetH)

	for y := range targetH {
		for x := range targetW {
			// Sample region in source image
			x0 := int(float64(x) * scaleX)
			y0 := int(float64(y) * scaleY)
			x1 := int(float64(x+1)*scaleX) + 1
			y1 := int(float64(y+1)*scaleY) + 1

			var r, g, b, a uint32
			count := 0

			for sy := y0; sy < y1 && sy < srcH; sy++ {
				for sx := x0; sx < x1 && sx < srcW; sx++ {
					c := img.At(sx, sy).(color.RGBA)
					r += uint32(c.R)
					g += uint32(c.G)
					b += uint32(c.B)
					a += uint32(c.A)
					count++
				}
			}

			if count > 0 {
				out.SetRGBA(x, y, color.RGBA{
					R: uint8(r / uint32(count)),
					G: uint8(g / uint32(count)),
					B: uint8(b / uint32(count)),
					A: uint8(a / uint32(count)),
				})
			}
		}
	}

	return out
}

func formatRGB(c color.RGBA) string {
	return fmt.Sprintf("\x1b[38;2;%d;%d;%d", c.R, c.G, c.B)
}

func kittyPlaceholders(cols, rows, id int) string {
	// Use kitty Unicode placeholder protocol
	// Placeholder: U+10EEEE followed by row/col diacritics
	var buf bytes.Buffer

	placementOpts := kitty.Options{
		Action:           kitty.Put,
		ID:               id,
		Columns:          cols,
		Rows:             rows,
		VirtualPlacement: true,
	}

	buf.WriteString("\x1b_G")
	for _, opt := range placementOpts.Options() {
		buf.WriteString(opt)
		buf.WriteString(",")
	}
	buf.WriteString("\x1b\\")

	// Write placeholder cells
	for row := range rows {
		for col := range cols {
			// Write placeholder with diacritics
			buf.WriteRune('\U0010eeee')
			buf.WriteRune(kitty.Diacritic(row))
			buf.WriteRune(kitty.Diacritic(col))
		}
		buf.WriteString("\r\n")
	}

	return buf.String()
}

func base64EncodeChunk(data []byte) string {
	// Simple base64 encoding
	const charset = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789+/="
	var buf bytes.Buffer

	for i := 0; i < len(data); i += 3 {
		b1 := data[i]
		var b2, b3 byte
		if i+1 < len(data) {
			b2 = data[i+1]
		}
		if i+2 < len(data) {
			b3 = data[i+2]
		}

		buf.WriteByte(charset[b1>>2])
		buf.WriteByte(charset[((b1&3)<<4)|(b2>>4)])

		if i+1 < len(data) {
			buf.WriteByte(charset[((b2&15)<<2)|(b3>>6)])
		} else {
			buf.WriteByte('=')
		}

		if i+2 < len(data) {
			buf.WriteByte(charset[b3&63])
		} else {
			buf.WriteByte('=')
		}
	}

	return buf.String()
}
