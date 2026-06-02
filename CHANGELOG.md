# Changelog

All notable changes to rift are documented here. The format is based on
[Keep a Changelog](https://keepachangelog.com/en/1.1.0/), and rift follows
[Semantic Versioning](https://semver.org/spec/v2.0.0.html) (loosely, while
pre-1.0).

## [Unreleased]

### Changed

- Reading mode (`⇥`) now collapses the list to a one-row peek of the current
  item above the focused diff, rather than hiding it entirely. File-boundary
  banners are no longer drawn in the body (the diff's own file header marks each
  file); instead the diff panel's border legend shows the current file's name on
  the left (updating as you scroll between files) and its position — file `N/M`
  of the changeset — on the right. The footer hint reads `⇥ list`.
- The diff browser's working-tree list title now shows both modes as a toggle —
  `unstaged/staged` with the active one highlighted — instead of only the current
  one, so the `s` toggle and the current state read at a glance.

## [0.3.0] - 2026-06-01

### Added

- `J` / `K` (also `shift+↑`/`↓`, or `]`/`[`) step to the next/previous file or
  commit without leaving the preview, so you can read straight through a
  changeset file by file — a bigger-unit jump complementing the `j`/`k` scroll.

### Changed

- Diffs now load progressively: the "all changes", commit, and stash previews
  diff their files in parallel but stream in top-to-bottom, so the first file
  (and, for a commit, its header) paints as soon as it's ready instead of
  blocking on the whole changeset (difftastic is CPU-bound at roughly a second
  per large file). The full result is cached, so revisiting is instant.
- The diff, log, and stash browsers now use a vertical layout: the list sits in
  a strip on top (capped at ~⅓ of the height) with the diff filling the rest
  below at full width — instead of a left sidebar that competed with the diff
  for columns. Stepping the list updates the diff live; `⇥` focuses the diff
  full-screen (the strip hides, the current file/commit and position move into
  its title) and back.

### Fixed

- Renamed files no longer show an empty diff in stash and linked-worktree
  commit previews (`git diff --name-status` rename rows were misparsed).

## [0.2.1] - 2026-06-01

### Fixed

- Diff previews in the log browser now render correctly inside linked
  worktrees and for root (parentless) commits, instead of coming up empty.
- Failed shelled-out diffs now surface git's stderr in the error message,
  so the cause is visible instead of a bare exit code.

## [0.2.0] - 2026-05-31

A big step toward being a daily driver for *reading* git state: the diff and log
browsers got a top-to-bottom presentation overhaul, and the tool's scope
tightened around what it does best.

### Added

- Colored file-type icons, emphasized filenames over dimmed directories, and
  display-width-aware path truncation make file lists scan instantly.
- Scrollbars, a sticky file header, and file-boundary banners keep you oriented
  in long, multi-file diffs.
- Log drill-down: press `⏎` to open a commit's changed files.
- Cherry-pick (`c`) and revert (`r`) the selected commit from the log; scope
  history with ranges like `main..HEAD` or `@{upstream}..`.
- `o` opens the selected file in `$EDITOR` at the change, `y` yanks the
  selection, and `?` shows a per-screen keybinding overlay.
- git-diff fallback engine with word-level and whitespace highlighting, plus an
  in-TUI engine toggle (`e`); difftastic `--tab-width` and language detection
  for extensionless files (Makefile, etc.).
- Width-aware diff layout with an inline ↔ side-by-side toggle (`\`), per-file
  `+/-` stats, and commit bodies rendered as lightweight markdown.
- Preview caching with a loading spinner.

### Changed

- `esc` now means "go back" (exit a drill-down, clear a filter) instead of
  quitting; quit stays on `q` / `ctrl+c`.
- Upgraded to the Charm v2 ecosystem (bubbletea / lipgloss / bubbles) and
  extracted a reusable split-list component shared across screens.
- Repositioned as *worktree-aware* — every command reads correctly inside
  bare-repo and linked-worktree layouts — rather than worktree-managing. Pair
  with [worktrunk](https://github.com/max-sixty/worktrunk) for managing
  worktrees and branches.

### Removed

- The `branch` command. rift no longer manages branches or worktrees.

[Unreleased]: https://github.com/madhermit/rift/compare/v0.3.0...HEAD
[0.3.0]: https://github.com/madhermit/rift/compare/v0.2.1...v0.3.0
[0.2.1]: https://github.com/madhermit/rift/compare/v0.2.0...v0.2.1
[0.2.0]: https://github.com/madhermit/rift/compare/v0.1.2...v0.2.0
