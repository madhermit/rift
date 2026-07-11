# CLAUDE.md — rift

## Project Overview

rift is a syntax-aware, worktree-aware, composable fuzzy git tool focused on consuming git state (diff, log, stash, stage) — fast, transient, scriptable. Single Go binary. See `git-flux-design.md` for full design doc.

rift does **not** manage worktrees or branches (no create/switch/prune) — defer that to [worktrunk](https://github.com/max-sixty/worktrunk). rift's job is to read and present what's there, correctly, including inside bare-repo / linked-worktree layouts.

## Build & Dev Commands

```bash
mise run build       # build binary → ./rift
mise run test        # go test ./...
mise run fmt         # go fmt ./...
mise run lint        # go vet ./...
mise run check       # fmt + lint + test
mise run install     # install to ~/.local/bin/rift
```

## Tech Stack

- **Language:** Go 1.25
- **TUI:** bubbletea / lipgloss / bubbles (Charm ecosystem)
- **Git reads:** go-git
- **Git writes:** shelled git commands
- **Structural diff:** difftastic (external binary, shelled out)
- **Test extraction:** tree-sitter grammars (gotreesitter, embedded) for the `--tests` lens
- **Task runner:** mise

## Project Structure

```
cmd/           # cobra command definitions (one file per subcommand)
internal/      # private packages
  git/         # git operations (go-git reads, shelled writes)
  tui/         # bubbletea models and views
  diff/        # difftastic integration, fallback line diff
  review/      # review state (reviewed marks) + test-case extraction
  tooling/     # external tool management (difftastic download/detect)
  output/      # --print / --json composable output
  skill/       # embedded agent skill (SKILL.md) + materialization
main.go        # entrypoint
```

## Planned (not yet built)

`git-flux-design.md` describes far more than ships today. The following are
design intent only — do not assume the packages or tools exist:

- **Config:** TOML at `~/.config/rift/config.toml` + per-repo `.rift.toml` (`internal/config`)
- **Syntax-aware merge / conflict resolution:** mergiraf (`internal/merge`)
- **Checkpoints:** shadow-commit snapshots on hidden refs (`internal/checkpoint`)
- **Risk classification** in review — today `internal/review` only tracks reviewed marks and extracts test cases
- **Inline agent annotations:** `rift annotate` writing to a JSON store next to the reviewed marks (`.git/rift/annotations.json`), rendered in the diff preview and picked up by `--watch`. File-based on purpose — no daemon/socket (see the design doc's Future Considerations)
- **Mouse support:** wheel scroll + click-to-select via bubbletea mouse events; must keep terminal text selection workable (see design doc)

## Workflow

- **Refactoring pass required.** After implementing any feature or change, always do a refactoring pass before considering the work done. Review every file touched for: dead code (unused fields, methods, variables, imports), duplication (extract shared helpers), unnecessary indirection (methods that don't use their receiver should be plain functions), and structural simplicity (e.g. lift common logic above a switch instead of duplicating it in each branch). The code should be as simple and idiomatic as possible.

## Commits

- **No AI attribution.** Do not add a `Co-Authored-By` trailer or any AI/assistant attribution to commit messages.
- **Changelog.** For user-facing changes (new/removed commands or flags, behavior changes, notable UX), add a bullet under `## [Unreleased]` in `CHANGELOG.md` in the right category (Added / Changed / Removed / Fixed). At release time, rename `[Unreleased]` to the new `[x.y.z] - YYYY-MM-DD`, add the compare link, and start a fresh empty `[Unreleased]`. Purely internal changes (refactors, test-only, tooling) don't need an entry.

## Code Conventions

- **Error handling:** Return errors, don't panic. Use `fmt.Errorf("context: %w", err)` for wrapping.
- **Naming:** Follow Go conventions. Packages are short, lowercase, singular nouns. No `utils` or `helpers` packages.
- **Interfaces:** Define interfaces where they're consumed, not where they're implemented.
- **Testing:** Table-driven tests. Use `testify` only if already in deps; prefer stdlib `testing`.
- **Comments:** Only where the "why" isn't obvious. No doc comments on unexported functions unless the logic is non-trivial.
- **Linting:** `go vet` is the baseline. No other linters are configured yet.

## Architecture Rules

- **Internal packages only.** Nothing under `internal/` is public API. The CLI is the interface.
- **No global state.** Pass dependencies explicitly. No `init()` functions except for cobra command registration.
- **Composable output on every command.** Every subcommand must support `--print` and `--json` flags. Use `internal/output` for consistent formatting.
- **Graceful degradation.** If difftastic is unavailable, fall back to the built-in line diff. Never crash on missing external tools.
- **Worktree awareness, not management.** Commands detect and respect worktree context (bare-repo and linked-worktree layouts) so reads work correctly there; the logic lives in `internal/git` (e.g. `isLinkedWorktree`, the `log.go` commondir shell fallback). rift never creates, switches, or prunes worktrees — that's worktrunk's job.

## External Tool Management

- difftastic (`difft`) is auto-downloaded to `~/.local/share/rift/bin/` on first run
- A system `$PATH` version is preferred if present and version-compatible
- Fallback to built-in line diff if difftastic is unavailable
- Never block on a failed download

## Common Pitfalls

- Don't use `os.Exit()` except in `main()`. Return errors up the call stack.
- Don't shell out to git for read operations — use go-git. Shell out only for writes (commit, push, merge, rebase) where go-git support is incomplete.
- Don't render TUI escape codes when stdout is not a TTY — auto-switch to `--print` mode.
- Don't hardcode color — respect `NO_COLOR` env var and terminal capability detection.
