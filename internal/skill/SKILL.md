---
name: rift-review
description: Review git changes with rift — structural diffs, test-impact analysis, and review tracking through composable --print/--json output. Use when reviewing a diff or commit, checking which tests a change touches, or coordinating a review with a human in a repo where rift is installed.
---

# Reviewing changes with rift

rift is a git tool built for reading code: syntax-aware diffs (via difftastic),
a test-impact lens, and per-file review tracking. Every command works two ways —
an interactive TUI for humans, and `--print` / `--json` output for you. This
skill covers the composable side; never try to drive the TUI.

Ground rules:

- Always pass `--print` or `--json` explicitly. (When stdout is not a TTY rift
  falls back to `--print` on its own, so it will never hang you, but being
  explicit keeps intent clear.)
- rift only reads and presents git state — it never mutates the working tree,
  and the only writes it can make happen through human keypresses in the TUI
  (stage, stash apply, cherry-pick). Running the commands below is always safe.
- All commands accept a trailing `-- <path>...` to scope to files or
  directories.

## Inspect a changeset

```bash
rift diff --json                   # unstaged changes: [{path, status, added, deleted}, ...]
rift diff --staged --json          # staged changes
rift diff main --json              # working tree vs a ref (committed + uncommitted)
rift diff HEAD~3 HEAD --json       # a committed range
rift diff --name-only              # just the paths, one per line
```

`status` is `Modified`, `Added`, `Deleted`, `Renamed`, `Copied`, or `Untracked`.
In a commit-range or base-ref scope, `Renamed` entries also carry `old_path`.

## Read the diffs

```bash
rift diff --print                  # every file's diff, structural when difftastic is available
rift diff --print -- cmd/          # scoped to a directory
```

Structural diffs elide reformatting noise and show expression-level changes —
usually much less text than `git diff` for the same change.

## Test impact

```bash
rift diff --tests --json           # the test cases the diff touches
rift log HEAD~5 --tests --json     # same, for a commit range
```

Output: `[{file, language, name, line, status, path?, ticket?}, ...]` where
`status` is `added`, `renamed`, or `modified`. Pay closest attention to
`modified`: an existing test whose body changed can mean a quietly weakened
assertion. A change that touches no tests is also a finding worth reporting.
Extraction covers Go, JavaScript/TypeScript, Ruby, Python, and Rust, including
subtests and table-driven cases.

## Review state

Humans mark files reviewed in the rift TUI (`r`). A mark is keyed to the file's
content hash, so it resets the moment the file changes again. You can read that
state to know what still needs attention:

```bash
rift diff --unreviewed --name-only   # files the human has not (re-)reviewed
rift diff --unreviewed --json
```

If you edit a file that was already marked reviewed, its mark resets — that is
expected and correct; don't try to avoid it.

## History

```bash
rift log --json -n 20              # [{hash, author, date, message, body?}, ...]
rift log main..HEAD --json         # commits on this branch only
rift log --json -- internal/git    # commits touching a path
```

## Reviewing alongside a human

The human may have `rift diff --watch` open in another terminal: it live-reloads
as you edit, and their reviewed marks tick off (or reset) as files change. You
don't need to signal anything — just edit; rift picks it up. A useful summary
when you finish a change:

```bash
rift diff --json                   # what changed
rift diff --tests --json           # which tests the change touches
rift diff --unreviewed --name-only # what the human hasn't looked at yet
```

## A suggested review pass

1. `rift diff <scope> --json` — map the changeset; note sizes and statuses.
2. `rift diff <scope> --tests --json` — check the test story: are there
   `modified` tests to scrutinize? Is new code untested?
3. `rift diff <scope> --print` — read the structural diffs, largest files first.
4. Report findings referencing `path:line` from the JSON.
