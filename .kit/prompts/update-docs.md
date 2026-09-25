---
description: Update README, ENGINE.md, in-app help and godoc for a recent change
---

Review recent code changes, identify every documentation surface that should mention them, and update each one. Base the updates on the actual diff, not on guesses.

## Steps

1. **Identify the change**:
   - If the user input ($@) names a commit, PR, branch or topic, focus on that
   - Otherwise, on a feature branch, inspect `git log origin/master..HEAD --oneline` and `git diff origin/master...HEAD --stat`. On master, use the most recent commits (`git log -10 --oneline`)
   - Read the actual diff. Never document features that aren't in the code

2. **Inventory the doc surfaces** for this repo:
   - `README.md`: the user-facing surface, with sections for features, install, usage, the flag table, keys, settings, terminal support (renderer and kitty placement tables) and supported syntax per diagram type
   - `docs/ENGINE.md`: the contributor guide: pipeline, package table, how to write a diagram type, conventions, and terminal output internals. Update it when package responsibilities, invariants or workflows change
   - **In-app help**: the `keyMap` bindings (`key.WithHelp`) and `ShortHelp`/`FullHelp` in `internal/tui/styles.go`, the status bar (`statusView` in `internal/tui/model.go`) and the theme picker hints (`internal/tui/themepicker.go`)
   - **CLI help**: the cobra `Short`/`Long`/`Example`/flag descriptions and subcommands (`types`, `themes`) in `main.go` (rendered by fang)
   - **install.sh**: its `usage()` text and the header comment, if install options or supported platforms changed (keep them in sync with `.goreleaser.yaml`)
   - **Doc comments** on changed or new exported symbols
   - `examples/<type>/*.mmd` and `examples/showcase.md`, if the change deserves a demo diagram

3. **Audit each surface** with grep:
   - Search for the names of related keys, flags, themes, diagram keywords and functions to find every place that already discusses the area
   - Decide for each hit whether it needs an update, a cross-reference, or no change

4. **Draft the updates**:
   - Match each surface's existing voice and format (README tables for flags, keys and terminals; `key.NewBinding(... key.WithHelp(...))` for help)
   - Keep key and flag names consistent everywhere: the README, `WithHelp`, status bar hints and CLI help must agree
   - Verify every command, key, flag and identifier against the source files

5. **Verify**:
   - `go vet ./...` and `go build ./...` if Go files changed (help text lives in code)
   - Check the in-app help renders without overflowing: open it with `?` in tmux at 80 and 150 columns (see /tui-check)
   - `go run . --help` if CLI help changed
   - Run every example command you added to the README

6. **Report**:
   - List every file changed, and every surface deliberately left alone with a one-line reason
   - Suggest the next step (usually /commit-push with a `docs:` subject). Do not commit unless asked

## Guidelines

- Read the diff before writing anything. Invented key names or flags erode trust faster than missing docs
- Keep doc updates in their own commit, separate from code changes, where practical
- Prefer linking between README sections over duplicating content

$@
