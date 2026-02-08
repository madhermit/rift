# CLAUDE.md — rift

## Project Overview

rift is a syntax-aware, worktree-native, composable fuzzy git tool. Single Go binary. See `git-flux-design.md` for full design doc.

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
- **Syntax-aware merge:** mergiraf (external binary, shelled out)
- **Config:** TOML (`~/.config/rift/config.toml`, per-repo `.rift.toml`)
- **Task runner:** mise

## Project Structure

```
cmd/           # cobra command definitions (one file per subcommand)
internal/      # private packages
  git/         # git operations (go-git reads, shelled writes)
  tui/         # bubbletea models and views
  diff/        # difftastic integration, fallback line diff
  merge/       # mergiraf integration
  review/      # risk classification, review state
  checkpoint/  # shadow commit checkpoint system
  worktree/    # worktree management
  config/      # TOML config loading
  output/      # --print / --json / --format composable output
main.go        # entrypoint
```

## Workflow

- **Refactoring pass required.** After implementing any feature or change, always do a refactoring pass before considering the work done. Review every file touched for: dead code (unused fields, methods, variables, imports), duplication (extract shared helpers), unnecessary indirection (methods that don't use their receiver should be plain functions), and structural simplicity (e.g. lift common logic above a switch instead of duplicating it in each branch). The code should be as simple and idiomatic as possible.

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
- **Graceful degradation.** If difftastic/mergiraf are unavailable, fall back to built-in alternatives. Never crash on missing external tools.
- **Worktree awareness everywhere.** Commands should detect and respect worktree context. Use `internal/worktree` for shared logic.

## External Tool Management

- difftastic (`difft`) and mergiraf are auto-downloaded to `~/.local/share/rift/bin/` on first run
- System `$PATH` versions are preferred if present and version-compatible
- Fallback to built-in line diff if difftastic is unavailable
- Never block on a failed download

## Common Pitfalls

- Don't use `os.Exit()` except in `main()`. Return errors up the call stack.
- Don't shell out to git for read operations — use go-git. Shell out only for writes (commit, push, merge, rebase) where go-git support is incomplete.
- Don't render TUI escape codes when stdout is not a TTY — auto-switch to `--print` mode.
- Don't hardcode color — respect `NO_COLOR` env var and terminal capability detection.
