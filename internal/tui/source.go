package tui

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/mark3labs/mergo/internal/diagram"
)

// Diagram is one Mermaid diagram loaded from an input.
type Diagram struct {
	// Path of the file it came from ("-" for stdin).
	Path string
	// Block is the index of the mermaid block inside a markdown file (0 for
	// plain .mmd files).
	Block int
	// Line is the 1-based line in the file where the diagram source starts.
	Line int
	// Source is the Mermaid source.
	Source string
}

// Name returns a short human readable name.
func (d *Diagram) Name() string {
	base := filepath.Base(d.Path)
	if d.Path == "-" {
		base = "stdin"
	}
	if d.Block > 0 || isMarkdown(d.Path) {
		return fmt.Sprintf("%s #%d", base, d.Block+1)
	}
	return base
}

// Kind returns the detected diagram type (or "?" if unknown).
func (d *Diagram) Kind() string {
	doc, err := diagram.Preprocess(d.Source)
	if err != nil || doc == nil || doc.Type == "" {
		return "?"
	}
	return doc.Type
}

func isMarkdown(p string) bool {
	switch strings.ToLower(filepath.Ext(p)) {
	case ".md", ".markdown", ".mdx":
		return true
	}
	return false
}

// Block is a fenced mermaid code block.
type Block struct {
	Line   int // 1-based line of the first source line
	Source string
	// Lines is the number of source lines (between the fences).
	Lines int
	// Indent is the indentation of the opening fence, removed from the
	// source lines (CommonMark).
	Indent int
}

// ExtractMermaidBlocks returns all ```mermaid (or ~~~mermaid) fenced blocks
// in a markdown document.
func ExtractMermaidBlocks(md string) []Block {
	var out []Block
	sc := bufio.NewScanner(strings.NewReader(md))
	sc.Buffer(make([]byte, 1024*1024), 16*1024*1024)
	var (
		in     bool
		fence  string
		indent int
		start  int
		buf    []string
		lineNo int
	)
	for sc.Scan() {
		lineNo++
		line := sc.Text()
		trimmed := strings.TrimLeft(line, " ")
		ind := len(line) - len(trimmed)
		if !in {
			if ind > 3 {
				continue
			}
			for _, f := range []string{"```", "~~~"} {
				if strings.HasPrefix(trimmed, f) {
					n := len(trimmed) - len(strings.TrimLeft(trimmed, f[:1]))
					info := strings.TrimSpace(trimmed[n:])
					lang := strings.ToLower(strings.Fields(info + " x")[0])
					lang = strings.Trim(lang, "{}.")
					if lang == "mermaid" || lang == "mmd" {
						in = true
						fence = strings.Repeat(f[:1], n)
						indent = ind
						start = lineNo + 1
						buf = buf[:0]
					}
					break
				}
			}
			continue
		}
		if strings.HasPrefix(trimmed, fence) && strings.TrimSpace(strings.TrimLeft(trimmed, fence[:1])) == "" {
			out = append(out, Block{Line: start, Source: strings.Join(buf, "\n"), Lines: len(buf), Indent: indent})
			in = false
			continue
		}
		// remove up to `indent` spaces of indentation (CommonMark)
		rm := min(indent, ind)
		buf = append(buf, line[rm:])
	}
	if in && len(buf) > 0 {
		out = append(out, Block{Line: start, Source: strings.Join(buf, "\n"), Lines: len(buf), Indent: indent})
	}
	return out
}

// LoadInputs reads the given paths (or stdin for "-") and returns all
// diagrams. stdin is read at most once.
func LoadInputs(paths []string, stdin io.Reader) ([]*Diagram, error) {
	var out []*Diagram
	for _, p := range paths {
		ds, err := loadPath(p, stdin)
		if err != nil {
			return nil, err
		}
		out = append(out, ds...)
	}
	return out, nil
}

func loadPath(p string, stdin io.Reader) ([]*Diagram, error) {
	var data []byte
	var err error
	if p == "-" {
		if stdin == nil {
			stdin = os.Stdin
		}
		data, err = io.ReadAll(stdin)
	} else {
		data, err = os.ReadFile(p)
	}
	if err != nil {
		return nil, err
	}
	return parseInput(p, string(data)), nil
}

// parseInput splits file content into diagrams. Markdown files (by
// extension, or stdin content that contains a mermaid fence) contribute
// one diagram per mermaid block.
func parseInput(p, content string) []*Diagram {
	md := isMarkdown(p)
	if !md && p == "-" && (strings.Contains(content, "```mermaid") || strings.Contains(content, "~~~mermaid")) {
		md = true
	}
	if md {
		var out []*Diagram
		for i, b := range ExtractMermaidBlocks(content) {
			out = append(out, &Diagram{Path: p, Block: i, Line: b.Line, Source: b.Source})
		}
		return out
	}
	return []*Diagram{{Path: p, Line: 1, Source: content}}
}

// modTimes returns the modification times of all real files.
func modTimes(paths []string) map[string]time.Time {
	out := map[string]time.Time{}
	for _, p := range paths {
		if p == "-" {
			continue
		}
		if st, err := os.Stat(p); err == nil {
			out[p] = st.ModTime()
		}
	}
	return out
}
