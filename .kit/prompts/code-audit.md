---
description: Read-only audit for dead code, duplication, boundary violations and refactors
---

Perform a thorough **read-only** audit of this repository and report the findings. **Do not edit, rename, or delete any files.** Optional focus or scope hints from the user: $@

## Scope

If the user gave focus hints above (a package such as `internal/tui`, a diagram type such as `sequence`, or a concern such as "layout" or "themes"), scope the audit to that. Otherwise audit the whole repo, starting with the largest and busiest packages: `internal/diagrams/*` (flowchart, sequence, class, state, gantt first), then `internal/tui`, `internal/layout`, `internal/scene`, `internal/diagram`, `internal/theme`, `internal/mermaid`, `internal/config`, and `main.go`.

## Steps

1. **Map the repo first**:
   - List every Go package and its size (`find . -name '*.go' | xargs wc -l | sort -n`)
   - Read `docs/ENGINE.md` and `README.md` to learn the intended pipeline, package boundaries and conventions. These define what counts as a violation

2. **Hunt for dead code**:
   - Run `go vet ./...` and `golangci-lint run ./...` and capture their output
   - If `deadcode` is available (`go run golang.org/x/tools/cmd/deadcode@latest ./...`), run it and include its output verbatim. Remember `cmd/mmdpng` and tests are legitimate callers
   - Everything is under `internal/`, so "exported" doesn't mean "public". Grep exported and unexported symbols and cross-check call sites; symbols with no non-test references are suspects
   - Check for unused theme fields and helpers in `internal/theme`, unused scene primitives or markers in `internal/scene`, and key bindings in `keyMap` (`internal/tui/styles.go`) that nothing matches
   - Look for commented-out blocks, `// TODO` markers, and pointless `_ = x` discards
   - **Do not delete anything.** List candidates with file:line and a confidence level (high / medium / low)

3. **Find unnecessary duplication**:
   - Likely hot spots: per-diagram parsers re-implementing line splitting, quoting, `classDef`/`style` handling or `%%` comment stripping that `internal/diagram` already offers; per-diagram label measurement and wrapping vs `diagram.LabelSize` / `scene.WrapText`; node shape drawing shared between flowchart, state and class; legend and axis code shared between pie, xychart, quadrant and gantt; color math duplicated outside `internal/theme`
   - Tell *coincidental* duplication (things that look alike but will change independently, e.g. two diagram types that follow different Mermaid rules) from *unnecessary* duplication (same intent, has to change in lockstep). Flag only the latter
   - For each cluster, suggest where a shared helper should live (usually `internal/diagram` or `internal/scene`) and whether that would cross a package boundary

4. **Check boundary violations** against docs/ENGINE.md:
   - The pipeline is one-way: `diagram` → `diagrams/<type>` → `scene` → image → `tui`. `internal/scene`, `internal/layout` and `internal/theme` must not import `internal/diagram`, any `internal/diagrams/*`, or `internal/tui`
   - Diagram packages must not import each other or `internal/tui`, and must render only to `*scene.Scene` (no direct rasterizing, no terminal I/O)
   - `internal/mermaid` is the only place diagram packages are linked in (`register_<type>.go` blank imports)
   - Only `internal/tui` (and `main.go`) touch the terminal and Charm libraries
   - `main.go` should only wire things together; business logic belongs in `internal/`
   - Conventions: parsers never panic (look for unchecked indexing / slicing on user input); text measured only via `scene.MeasureText` / `scene.MeasureBlock`; labels through `diagram.CleanLabel`
   - For each violation, cite the offending import or code with file:line

5. **Spot refactor opportunities**:
   - Long functions (>80 lines) doing several unrelated things. Likely candidates: the per-type `Render` and parse functions, `Model.Update` / `handleKey` / `View` in `internal/tui/model.go`
   - Deeply nested conditionals that early returns would flatten
   - Structs with many fields that suggest split responsibilities (`tui.Model` is a candidate: group camera, picker and image state?)
   - Test setup boilerplate that a helper could replace
   - For each: location, current shape (1–2 lines), proposed shape (1–2 lines), and risk (low / medium / high)

6. **Cross-check against project rules**: re-read docs/ENGINE.md and drop any "refactor" that would break a documented invariant, for example rotating fewer kitty image ids, or rasterizing the whole scene instead of the visible viewport. Say that you dropped it and why

7. **Write the report** as your final message (do not write it to disk):

   ```
   # Code Audit Report

   ## Summary
   - N dead-code candidates
   - N duplication clusters
   - N boundary violations
   - N refactor opportunities

   ## Dead Code
   ### High confidence
   - path/to/file.go:LINE — symbol — reason
   ### Medium confidence
   …

   ## Duplication
   ### Cluster: <short name>
   - Sites: file:line, file:line, …
   - Suggested home: package/path
   - Notes: …

   ## Boundary Violations
   - Rule: <which ENGINE.md rule or convention>
   - Offender: file:line
   - Fix sketch: …

   ## Refactor Opportunities
   - Location: file:line
   - Current: …
   - Proposed: …
   - Risk: low/medium/high
   - Why it's worth it: …

   ## Suggested Next Steps
   1. …
   ```

8. **End the report with an explicit reminder** that no files were modified. Recommend picking the highest-value items to act on one at a time (for example via /file-issue then /fix-issue) rather than a sweeping refactor

## Guidelines

- **Read-only, always**: no edits, no writes, no `git commit`, no `go mod tidy`. Use only read, grep, find, ls, and read-only commands (`go vet`, `go build -o /tmp/...`, `golangci-lint run`, `deadcode`)
- **Cite every finding** with `path/to/file.go:LINE`
- **Be honest about confidence**: false positives are expensive, so prefer "medium confidence, worth a look" over confidently wrong claims
- **Quality over quantity**: 10 sharp findings beat 100 nitpicks. Cut anything purely stylistic
- **Don't propose architectural rewrites**: recommend small, reviewable changes that fit the existing structure
