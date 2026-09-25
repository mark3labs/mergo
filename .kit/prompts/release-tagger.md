---
description: Semantic version tagging workflow: analyze commits and tag a release
---

# Release Tagging Workflow

Tag a new version of mergo following semantic versioning.

## Steps

1. **Make sure you're releasing master**:
   - `git switch master && git pull --ff-only`
   - The working tree must be clean (`git status --short` prints nothing)
   - Run the full checks: `gofmt -l .`, `go vet ./...`, `golangci-lint run ./...`, `go test -race ./...`. Do not tag a red build
   - Check that CI on master is green: `gh run list --branch master --limit 3`
   - Optionally, dry-run the release locally: `goreleaser release --snapshot --clean` (then `rm -rf dist`)

2. **Fetch remote tags**: `git fetch --tags origin`

3. **Find the latest version**: `git tag -l 'v*' | sort -V | tail -5`
   - **No tags yet?** This is the first release. Propose `v0.1.0` (mergo is pre-1.0), list the highlights of the whole history, and skip the bump analysis

4. **Analyze changes since the last tag**:
   - `git log <latest-tag>..HEAD --oneline`
   - `git diff <latest-tag>..HEAD --stat`

5. **Determine the version bump** (semver; while < v1.0.0, breaking changes bump MINOR):
   - **MAJOR / breaking**: `BREAKING CHANGE:` footers or `!` subjects, removed or renamed CLI flags, subcommands or themes, an incompatible config file (`config.json`) change, removed keybindings, dropped Mermaid syntax that used to render
   - **MINOR**: `feat:` commits, new diagram types, new syntax support, flags, themes, keybindings or mouse interactions
   - **PATCH**: `fix:`, `perf:`, and `refactor:` with no visible change
   - **Skip**: `docs:`, `test:`, `ci:`, `chore:` only. Suggest not releasing
   - When in doubt, prefer the smaller bump

6. **Calculate the new version**: increment the chosen segment and reset lower segments to 0

7. **Draft the tag message**, grouped by type. (The GitHub release notes are generated separately by GoReleaser from commit subjects; the tag message is for `git show`.)

   ```
   v0.2.0 - themes with light/dark variants and a theme picker

   Features:
   - 23 themes, each with a light and a dark variant
   - Theme picker in the viewer with live preview, saved to config.json

   Fixes:
   - Kitty direct placement under zellij
   ```

8. **Wait for the user to confirm** the version and message before running any tag commands

9. **Create and push an annotated tag**:
   - `git tag -a vX.Y.Z -F /tmp/tag-msg.txt` (write the message to a file so multi-line bodies survive)
   - `git push origin vX.Y.Z`
   - Pushing the tag triggers `.github/workflows/release.yml`: tests, then GoReleaser publishes the Linux/macOS (amd64, arm64) tarballs and checksums that `install.sh` downloads. Watch it with `gh run watch` and confirm the release page lists 4 archives plus `mergo_X.Y.Z_checksums.txt`
   - Once published, both `curl -fsSL https://raw.githubusercontent.com/mark3labs/mergo/master/install.sh | bash` and `go install github.com/mark3labs/mergo@vX.Y.Z` install it, and `mergo --version` reports `X.Y.Z`

## Guidelines

- Always fetch remote tags first to avoid conflicts
- Always use annotated tags (`-a`) with descriptive messages
- If there are no changes since the last tag, suggest skipping the release

$@
