# Changelog

All notable changes to rift are documented here. The format is based on
[Keep a Changelog](https://keepachangelog.com/en/1.1.0/), and rift follows
[Semantic Versioning](https://semver.org/spec/v2.0.0.html) (loosely, while
pre-1.0).

## [Unreleased]

### Added

- Mark changes reviewed in `rift diff`: press `r` to tick off the selected file
  (a ✓ replaces its status glyph) and `u` to show only the files you haven't
  reviewed yet — so you can walk a large change file by file and watch the list
  shrink. A mark is keyed to the file's content, so it resets the moment the file
  changes (e.g. an agent edits it again) and persists across runs, making it easy
  to re-review only what's new. `--unreviewed` narrows `--print` / `--json` /
  `--name-only` to the not-yet-reviewed files for scripting. Working-tree only;
  marks live in `.git/rift/` per worktree and aren't committed.

## [0.4.2] - 2026-06-04

### Fixed

- `rift diff` and `rift stage` no longer stall for tens of seconds in a repo with
  large gitignored subtrees (e.g. `node_modules`, `vendor`). The working-tree
  file listing now comes from `git status` directly instead of go-git, whose
  status walk descended into ignored directories instead of pruning them. As a
  result the listing also honors global / `core.excludesFile` ignore rules (so
  `.DS_Store` and the like no longer show up) and lists untracked files inside
  linked worktrees.

## [0.4.1] - 2026-06-04

### Fixed

- `rift diff` with no commit argument now diffs the unstaged view against the
  index (matching `git diff`), not against HEAD. A file staged as new no longer
  renders as a whole addition in the unstaged view — only its actual unstaged
  change is shown; the `s` staged view still diffs against HEAD.

## [0.4.0] - 2026-06-03

### Added

- `--tests` — a lens on the diff-producing commands that lists the test cases a
  diff touches instead of files, so you can read what was built from its specs.
  It extracts tests across Go (standard parser, including `t.Run` subtests and
  table-driven cases — keyed-struct, map, and positional tables) and — via tree-sitter — JS/TS
  (Jest/Vitest/Mocha `describe`/`it`), Ruby (RSpec and Rails/Minitest `test "…"`
  macros), Python (pytest/unittest), and Rust (`#[test]`/`#[tokio::test]`). Each
  case is prefaced with its nesting — `describe`/`context` blocks, and the
  enclosing class for Python/Rails test classes — plus any embedded ticket IDs.
  Only tests the diff actually touches are shown. Each is marked by how the diff
  changed it — `+` added, `→` renamed (its name changed), `~` modified (its body
  changed under an unchanged name); the `~` cases are the ones worth a close read,
  since that's where an assertion can be quietly weakened.
  - `rift diff` shows the file list and toggles to the tests view live with `t`
    (the header reads `rift ❯ diff ❯ tests` to show it's a lens); `--tests` just
    opens straight into it. Works on the working tree, `--staged`, or a `<commit>
    <commit>` range, with `--print`/`--json` for scripting. The diff preview
    scrolls to the selected test and labels it in the panel's border, so a
    body-only change stays named even when its declaration isn't in the diff;
    `o` opens it in `$EDITOR` at its line.
  - `rift log --tests` makes drilling into a commit (`⏎`) show the test cases it
    touched rather than its files.

## [0.3.1] - 2026-06-02

### Changed

- Reading mode (`⇥`) now collapses the list to a one-row peek of the current
  item above the focused diff, rather than hiding it entirely. File-boundary
  banners are no longer drawn in the body (the diff's own file header marks each
  file); instead the diff panel's border legend shows the current file's name on
  the left (updating as you scroll between files) and its position — file `N/M`
  of the changeset — on the right. The footer hint reads `⇥ list`.
- The top bar is now a chevron breadcrumb (`rift ❯ diff`) with the current branch
  shown alongside the engine on the right (`main · difftastic`), and a thin rule
  beneath it mirrors the footer. The list selection position (e.g. `11/14`) now
  rides in the list/peek title instead of the footer.
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

[Unreleased]: https://github.com/madhermit/rift/compare/v0.4.2...HEAD
[0.4.2]: https://github.com/madhermit/rift/compare/v0.4.1...v0.4.2
[0.4.1]: https://github.com/madhermit/rift/compare/v0.4.0...v0.4.1
[0.4.0]: https://github.com/madhermit/rift/compare/v0.3.1...v0.4.0
[0.3.1]: https://github.com/madhermit/rift/compare/v0.3.0...v0.3.1
[0.3.0]: https://github.com/madhermit/rift/compare/v0.2.1...v0.3.0
[0.2.1]: https://github.com/madhermit/rift/compare/v0.2.0...v0.2.1
[0.2.0]: https://github.com/madhermit/rift/compare/v0.1.2...v0.2.0
