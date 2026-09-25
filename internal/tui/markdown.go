package tui

import (
	"regexp"
	"strings"
)

var mermaidBlockRe = regexp.MustCompile("(?s)```mermaid\\s*\\n([^`]*)\\n```")

// ExtractMermaidBlocks extracts all mermaid code blocks from markdown.
func ExtractMermaidBlocks(md string) []string {
	matches := mermaidBlockRe.FindAllStringSubmatch(md, -1)
	var blocks []string
	for _, match := range matches {
		if len(match) > 1 {
			blocks = append(blocks, strings.TrimSpace(match[1]))
		}
	}
	return blocks
}
