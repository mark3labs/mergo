package tui

import (
	"encoding/base64"
	"fmt"
	"os"
	"strings"

	"github.com/charmbracelet/x/ansi/kitty"
)

// Kitty graphics protocol helpers.
//
// Interactive mode uses *Unicode placeholders*: the image is transmitted
// once and a virtual placement is created; the image is then displayed by
// writing a grid of U+10EEEE characters whose combining diacritics encode
// the row/column and whose foreground color encodes the image id. Because
// these are ordinary (width 1) cells, they flow through bubbletea's cell
// based renderer like any other text, so scrolling, redraws and overlays
// all just work.
//
// See https://sw.kovidgoyal.net/kitty/graphics-protocol/#unicode-placeholders

const kittyChunk = 4096

// maxPlaceholderCells is the number of row/column diacritics available.
const maxPlaceholderCells = 297

// kittyGraphics wraps an APC graphics command.
func kittyGraphics(ctrl string, payload string, tmux bool) string {
	s := "\x1b_G" + ctrl
	if payload != "" {
		s += ";" + payload
	}
	s += "\x1b\\"
	if tmux {
		return wrapTmux(s)
	}
	return s
}

// wrapTmux wraps an escape sequence in a tmux DCS passthrough.
func wrapTmux(s string) string {
	return "\x1bPtmux;" + strings.ReplaceAll(s, "\x1b", "\x1b\x1b") + "\x1b\\"
}

// kittyTransmit returns the escape sequences that transmit PNG data as image
// id (without displaying it). The payload is chunked.
func kittyTransmit(id int, png []byte, tmux bool) string {
	return kittyChunks(fmt.Sprintf("a=t,f=100,t=d,i=%d,q=2", id), png, tmux)
}

// kittyTransmitDisplay transmits and displays PNG data at the cursor,
// scaled into cols x rows cells (used by print mode).
func kittyTransmitDisplay(id int, png []byte, cols, rows int, tmux bool) string {
	return kittyChunks(fmt.Sprintf("a=T,f=100,t=d,i=%d,c=%d,r=%d,q=2", id, cols, rows), png, tmux)
}

func kittyChunks(ctrl string, data []byte, tmux bool) string {
	enc := base64.StdEncoding.EncodeToString(data)
	var sb strings.Builder
	sb.Grow(len(enc) + len(enc)/kittyChunk*32 + 64)
	first := true
	for len(enc) > 0 || first {
		n := min(kittyChunk, len(enc))
		chunk := enc[:n]
		enc = enc[n:]
		more := 0
		if len(enc) > 0 {
			more = 1
		}
		c := fmt.Sprintf("m=%d", more)
		if first {
			c = ctrl + "," + c
			first = false
		}
		sb.WriteString(kittyGraphics(c, chunk, tmux))
	}
	return sb.String()
}

// kittyVirtualPlacement creates a virtual placement for Unicode placeholders.
func kittyVirtualPlacement(id, cols, rows int, tmux bool) string {
	return kittyGraphics(fmt.Sprintf("a=p,U=1,i=%d,c=%d,r=%d,q=2", id, cols, rows), "", tmux)
}

// directZ is the z-index for direct placements: below cells with a
// non-default background (so overlays such as help and error panels cover
// the image) while still visible under default-background blank cells.
const directZ = -1073741825

// kittyPlaceAt places image id at the 0-based cell (row, col), scaled into
// cols x rows cells. The cursor position is saved and restored around the
// placement and the terminal is told not to move the cursor (C=1), so the
// sequence doesn't disturb the TUI renderer's idea of the cursor.
func kittyPlaceAt(id, row, col, cols, rows int, tmux bool) string {
	return "\x1b7" + fmt.Sprintf("\x1b[%d;%dH", row+1, col+1) +
		kittyGraphics(fmt.Sprintf("a=p,i=%d,p=1,c=%d,r=%d,C=1,z=%d,q=2", id, cols, rows, directZ), "", tmux) +
		"\x1b8"
}

// blankGrid returns rows lines of cols default-background spaces (the body
// under a direct placement).
func blankGrid(cols, rows int) []string {
	line := strings.Repeat(" ", max(cols, 0))
	out := make([]string, rows)
	for i := range out {
		out[i] = line
	}
	return out
}

// kittyDelete deletes an image (and frees its data).
func kittyDelete(id int, tmux bool) string {
	return kittyGraphics(fmt.Sprintf("a=d,d=I,i=%d,q=2", id), "", tmux)
}

// kittyQuery returns a query that a kitty-compatible terminal answers with
// an OK response for image id.
func kittyQuery(id int) string {
	return fmt.Sprintf("\x1b_Gi=%d,s=1,v=1,a=q,t=d,f=24;AAAA\x1b\\", id)
}

// placeholderGrid returns rows lines of cols placeholder cells for image id.
// The image id is encoded as a 256-color index in the foreground color,
// which survives color profile downsampling (ids must be 16..255).
func placeholderGrid(id, cols, rows int) []string {
	cols = min(cols, maxPlaceholderCells)
	rows = min(rows, maxPlaceholderCells)
	lines := make([]string, rows)
	fg := fmt.Sprintf("\x1b[38;5;%dm", id)
	for r := 0; r < rows; r++ {
		var sb strings.Builder
		sb.Grow(cols*10 + 16)
		sb.WriteString(fg)
		rd := string(kitty.Diacritic(r))
		for c := 0; c < cols; c++ {
			sb.WriteRune(kitty.Placeholder)
			sb.WriteString(rd)
			sb.WriteRune(kitty.Diacritic(c))
		}
		sb.WriteString("\x1b[39m")
		lines[r] = sb.String()
	}
	return lines
}

// imageIDBase picks a per-process base for image ids so that multiple
// instances in one terminal don't clash. Ids are kept in 64..253 so that
// they can be encoded as 256-color indices.
func imageIDBase() int {
	return 64 + (os.Getpid()%95)*2
}
