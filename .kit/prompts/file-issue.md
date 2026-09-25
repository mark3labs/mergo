---
description: File a GitHub issue (bug, feature, or docs) with a structured body
---

File a GitHub issue for the mergo repository. The user wants to create an issue about: $@

## Issue types

This repository has no issue templates, so give each issue the right **label** and a structured body:

| Type | Label | Title prefix | Use for |
|------|-------|--------------|---------|
| Bug | `bug` | `fix:` | Something is broken, renders wrong, or doesn't match Mermaid |
| Feature | `enhancement` | `feat:` | New feature, syntax support, diagram type, improvement |
| Docs | `documentation` | `docs:` | Missing, incorrect, or unclear documentation |

## Steps

1. **Determine the issue type** from the user input
2. **Investigate before writing**: grep the code to find the relevant package and file:line (see the package table in docs/ENGINE.md). For bugs, reproduce:
   - Rendering bugs: save a minimal diagram to `/tmp/repro.mmd` and run `go run ./cmd/mmdpng -ascii 120 /tmp/repro.mmd` (or render a PNG with `go run . -o /tmp/repro.png /tmp/repro.mmd`). Cut the diagram down to the smallest source that still shows the problem
   - Parse errors: `go run . -p /tmp/repro.mmd` shows the error message
   - Viewer bugs (keys, mouse, overlays, kitty vs half blocks): use a tmux capture (see /tui-check)
3. **Ask clarifying questions** only if critical information is missing and can't be found by investigating:
   - Bugs: "Which terminal, and which diagram source?"
   - Features: "What problem does this solve?"
   - Docs: "Where did you look for this information?"
4. **Craft the title**: `<prefix> <short description>`, lowercase, imperative mood, ≤72 chars
   - `fix: sequence notes overlap the next message label`
   - `feat: support the architecture-beta diagram type`
   - `docs: explain kitty placement modes under zellij`
5. **Write the body** to `/tmp/issue-body.md` using the matching structure:

   **Bug** (`bug`)
   ````markdown
   ## Bug Description
   What happened vs. what was expected (what mermaid.js renders, if relevant).

   ## Minimal Diagram
   ```mermaid
   …smallest source that reproduces it…
   ```

   ## Steps to Reproduce
   1. …

   ## Relevant Output
   Error message, ASCII preview, or a tmux capture of the screen.

   ## Affected Component
   parser/renderer for <type> | layout | scene | theme | tui | cmd | install

   ## Environment
   mergo version (`mergo --version` or `git rev-parse --short HEAD`), OS, terminal emulator (and multiplexer), renderer (`-r`) and theme
   ````

   **Feature** (`enhancement`)
   ```markdown
   ## Feature Description
   What to add or change. For syntax support, include example Mermaid source and a link to the Mermaid docs.

   ## Motivation / Use Case
   The problem it solves, the current workaround, and who benefits.

   ## Proposed Implementation (optional)
   High-level approach, affected packages, and example usage (flags, keys).
   ```

   **Docs** (`documentation`)
   ```markdown
   ## Documentation Issue
   What's wrong or missing.

   ## Documentation Location
   README.md section, docs/ENGINE.md, in-app help (`keyMap` in internal/tui/styles.go), CLI help (main.go), or godoc.

   ## Suggested Improvement
   How to fix it.
   ```

6. **Create the issue**:

       gh issue create --title "<title>" --label <label> --body-file /tmp/issue-body.md

7. **Confirm success**: show the issue URL, number, and label used

## Guidelines

- Include file paths and line numbers when you know them
- Keep the body factual. Keep speculation to the Proposed Implementation section
- For features, describe the problem first, then the solution. Keep mergo's design in mind: native Go rendering (no browser, no Node), Mermaid-compatible syntax, and a keyboard-driven viewer with mouse support
- If you're unsure about technical details, say so in the issue
