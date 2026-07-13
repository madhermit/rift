# Changelog

All notable changes to rift are documented here. The format is based on
[Keep a Changelog](https://keepachangelog.com/en/1.1.0/), and rift follows
[Semantic Versioning](https://semver.org/spec/v2.0.0.html) (loosely, while
pre-1.0).

## [Unreleased]

### Added

### Fixed

- `rift diff --watch` live-reloads the browser as the working tree changes —
  built for reviewing while an agent edits in another pane. The file list,
  stats, and diffs refresh in place (the selection survives), the tests lens
  re-collects, and content-keyed reviewed marks reset as files change, so the
  unreviewed filter always shows what still needs eyes. A `watching` tag in
  the header shows it's live, and `w` toggles it without restarting —
  enabling catches up on anything that changed while it was off.
- `rift skill` prints a built-in agent skill that teaches an AI agent rift's
  composable review workflow (`--json` changeset inspection, structural
  `--print` diffs, the `--tests` lens, review state). `rift skill path` writes
  it to `~/.local/share/rift/skills/rift-review/SKILL.md` and prints the path,
  so you can tell an agent "run `rift skill path` and follow that skill".
- Committed-range and base-ref diffs now detect renames: a moved file lists as
  one `R` record showing only its real content delta (with an `old → new`
  preview title and `old_path` in `--json`) instead of a full delete plus add.
  The tests lens reads a renamed file's old side from its old path, so its
  specs no longer all report as newly added. Working-tree and staged listings
  keep git-status semantics (the stage screen operates on real index paths).

- In a base-ref diff (`rift diff main`), returning from the editor (`o`) no
  longer replaces the file list with the plain worktree-vs-index set — the
  reload now honors the base ref.
- `install.sh` no longer fails checksum verification on macOS with a matching
  hash: it now compares hash strings directly instead of relying on `-c` check
  mode, whose behavior differs between GNU `sha256sum` and macOS's `shasum`.

## [0.6.0] - 2026-07-07

### Added

- Adaptive light/dark theme: rift now asks the terminal for its background color
  and picks a readable palette (previously the hardcoded dark palette was
  near-invisible on light terminals). `RIFT_THEME=light|dark` forces a palette;
  the default without an answer stays dark.
- `RIFT_ICONS=ascii` (or `none`) suppresses Nerd Font file icons for terminals
  without a patched font; the icon column drops out cleanly. The reviewed marker
  is now a standard `✓` in any font.
- Yank (`y`) falls back to an OSC 52 escape when no system clipboard is
  available, so copying works over SSH and in headless sessions.
- `rift stage` gains `y` (yank path), `e` (engine toggle), and `J`/`K` (or
  `]`/`[`) to step files while the diff pane is focused — matching the other
  screens.
- `gg`/`G` and `ctrl+d`/`ctrl+u` now also move the selection when the list pane
  is focused (previously they only scrolled the preview).
- Drilldowns now show where you are: a log drilldown is titled
  `log ❯ <hash>` (and `log ❯ <hash> ❯ tests` for the tests lens), with an
  `esc back` hint in the footer.
- The tests lens shows a "collecting tests…" note while it parses a large diff
  instead of appearing to ignore the keypress.
- `rift stage`'s file list supports the vim jump keys (`gg`/`G`,
  `ctrl+d`/`ctrl+u`, `ctrl+f`/`ctrl+b`) like every other list pane, and
  `ctrl+f`/`ctrl+b` now page through list panes (and the menu), not just the
  preview. The README gains a keybinding reference table.
- Releases now publish a `SHA256SUMS` file, and `install.sh` verifies the
  downloaded binary against it (degrading with a warning for older releases
  without one). A third-party license bundle is attached to releases as well.
- `install.sh` now points at shell completion (`rift completion bash|zsh|fish`),
  and the README documents completion, `rift log --all`, `-- <path>` scoping on
  `diff`/`log`, and `rift version --json`.

### Changed

- `esc` now consistently leaves the current mode (filter, help, confirmation,
  drilldown) and never quits a work screen — `rift stage` previously quit on
  esc. Quitting is `q` or `ctrl+c` everywhere.
- The `rift` launchpad menu is no longer one-shot: quitting a screen you opened
  from the menu returns to the menu. Screens that ran a git write (stash
  apply/pop/drop, cherry-pick/revert) still exit so git's output stays visible.
- Typing in the fuzzy filter keeps your selection when the selected item still
  matches, instead of resetting to the top on every keystroke.
- `rift log` ignores `c`/`r` while the preview pane is focused (matching
  stash's action keys), so reading a diff can't trigger a cherry-pick prompt.
- `rift stage` hunk navigation is now `{`/`}` — the same key that steps
  sections on every other screen; the extra `n`/`p` aliases were removed so `p`
  unambiguously means pop (stash). The menu's help overlay now documents its
  actual keys, including that `esc` quits the launchpad.
- `rift stash` now asks for an inline y/n confirmation before `p` (pop) and `x`
  (drop), and action keys only fire from the list pane. `rift log` likewise
  confirms `c` (cherry-pick) and `r` (revert) before running them.
- `rift diff --staged` now errors when combined with a commit argument instead of
  rendering a file list and diffs that disagree about the base.
- `rift log` now errors when given more than one commit ref instead of silently
  logging only the first.
- The editor for `o` is now resolved from `$VISUAL` before `$EDITOR` (the
  git/POSIX convention), and blank or whitespace-only values fall through to the
  next choice instead of crashing the TUI.
- difftastic is now installed from a pinned, checksum-verified release (was
  `latest`, unverified), written atomically, validated before use, and a failed
  download is remembered for an hour instead of stalling every command retrying.

### Fixed

- `rift diff <ref>` now lists the files that differ between the working tree and
  the ref — committed changes since the ref were previously invisible while each
  file's preview still diffed against it.
- `rift log --all` now works in bare-repo linked-worktree layouts (it previously
  printed nothing there), lists commits in commit-time order under `-n`, and
  includes tag-only and detached-HEAD commits.
- Non-ASCII and other quotable file paths are no longer dropped or shown
  C-quoted in diffs, hunk staging, the tests lens, and `--json` output, and they
  get accurate line stats.
- Untracked files now render in `rift diff --print` and in TUI previews on the
  plain git-diff engine (they were listed but showed no content).
- Deleted tracked files now render as a deletion diff in the TUI instead of
  "diff unavailable".
- Commit-range diffs (`rift diff A B`) now report real added/deleted line counts
  in `--json` and list rows instead of zeros.
- `rift diff --unreviewed` now applies interactively too (it opens the TUI with
  the unreviewed-only filter on), and `--name-only --unreviewed` filters
  identically on a TTY and piped.
- `rift diff --tests` no longer crashes on a bodyless Go test declaration, no
  longer lists test-shaped functions from production (non-test) files, surfaces
  tests weakened by deletion-only edits, no longer floods with false "added"
  tests when a test file is renamed, and shows nested `t.Run` subtests under
  their full path.
- Empty repos, non-repos, and bare git dirs now get clear errors ("no commits
  yet", "not a git repository…", "bare repository — run from a worktree"), and
  shelled-git failures include git's actual message instead of a bare exit
  status.
- Reviewed marks are written atomically and merged across concurrent rift panes
  instead of last-write-wins; a corrupt marks file is preserved as `.corrupt`
  rather than silently reset.
- In `rift stage`, a slow diff render can no longer land on the wrong file and
  stage a mismatched hunk; hunks stay in file order after staging (they no
  longer regroup by state); and renders are cached, so navigating and filtering
  no longer re-run difftastic on every keystroke.
- Switching panes with `⇥` no longer blanks the preview, restarts its diff, or
  loses the scroll position; large streamed previews no longer re-wrap the whole
  buffer on every chunk (progressively slower keystrokes on big changesets); and
  navigating to an already-cached item now cancels the superseded stream instead
  of letting it run difftastic to completion in the background.
- An empty changeset now shows "No changes found" instead of a blank "All
  changes" row; a menu filter with no matches says "no matches" instead of
  "1/0"; a failed commit-diff load in the log drilldown surfaces its error; and
  editing a file from `rift diff` refreshes the file list (a reverted file no
  longer lingers with stale stats).
- Fresh `go install` builds now report their module version from build info
  instead of "dev".

### Removed

- The unimplemented `--format` flag (it was advertised on every command and
  silently ignored). `--print` and `--json` are unchanged.

## [0.5.0] - 2026-06-08

### Added

- Mark changes reviewed in `rift diff`: press `r` to tick off the selected file
  (a ✓ replaces its status glyph) and `U` to show only the files you haven't
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

A big step toward being a daily driver for _reading_ git state: the diff and log
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
- Repositioned as _worktree-aware_ — every command reads correctly inside
  bare-repo and linked-worktree layouts — rather than worktree-managing. Pair
  with [worktrunk](https://github.com/max-sixty/worktrunk) for managing
  worktrees and branches.

### Removed

- The `branch` command. rift no longer manages branches or worktrees.

[Unreleased]: https://github.com/madhermit/rift/compare/v0.6.0...HEAD
[0.6.0]: https://github.com/madhermit/rift/compare/v0.5.0...v0.6.0
[0.5.0]: https://github.com/madhermit/rift/compare/v0.4.2...v0.5.0
[0.4.2]: https://github.com/madhermit/rift/compare/v0.4.1...v0.4.2
[0.4.1]: https://github.com/madhermit/rift/compare/v0.4.0...v0.4.1
[0.4.0]: https://github.com/madhermit/rift/compare/v0.3.1...v0.4.0
[0.3.1]: https://github.com/madhermit/rift/compare/v0.3.0...v0.3.1
[0.3.0]: https://github.com/madhermit/rift/compare/v0.2.1...v0.3.0
[0.2.1]: https://github.com/madhermit/rift/compare/v0.2.0...v0.2.1
[0.2.0]: https://github.com/madhermit/rift/compare/v0.1.2...v0.2.0
