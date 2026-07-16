# rift

Syntax-aware, worktree-aware, composable fuzzy git tool.

Most of a developer's time with code is no longer spent writing it — it's spent reviewing, parsing, and grokking changes that someone, or something, else wrote. rift is a git tool built for that shift.

It wraps the git workflows where UX is the bottleneck — diffing, staging, log browsing, stashing — with structural understanding via [difftastic](https://difftastic.wilfred.me.uk/) and composable output that works for both humans and scripts. And it treats review as a first-class workflow: mark files reviewed and have them re-surface only when their content changes again, and read a change by the tests it touches with the `--tests` lens — so a quietly deleted assertion in an agent-written diff has nowhere to hide.

```
rift              # contextual launchpad
rift diff         # syntax-aware diff browser
rift diff --tests # review a change by the tests it touches
rift diff --watch # live-reloading review while an agent edits
rift stage        # interactive staging with hunk granularity
rift log          # structural commit explorer
rift stash        # stash manager with diff preview
rift skill        # built-in skill that teaches agents the review workflow
```

rift is **worktree-aware** — every command reads correctly inside bare-repo and linked-worktree layouts — but it does not _manage_ worktrees or branches. For creating, switching, and pruning worktrees, pair it with [worktrunk](https://github.com/max-sixty/worktrunk).

<p align="center">
  <img src="docs/images/diff.gif" alt="rift diff — structural diff browser with a one-row peek and a per-file legend" width="900">
</p>

## Why

[forgit](https://github.com/wfxr/forgit) and [git-fuzzy](https://github.com/bigH/git-fuzzy) proved that fuzzy search over git objects is a massive UX win. But they're shell scripts piping strings through fzf — every diff is flat text, and you can't pipe the output into anything.

[lazygit](https://github.com/jesseduffield/lazygit) is the gold standard for git TUIs, but it's a resident app you live inside, with no composable output and line-based diffs.

rift occupies the space between them: **transient** (invoke, act, return to shell), **structural** (diffs understand your code's syntax), and **composable** (every command has `--print` and `--json` modes).

The bet behind it: as agents write more of the code, the scarce human activity becomes _reading_ it — so a git tool should be built for review first, and none of the above are.

### How it compares

A new cohort of review-first diff viewers — [hunk](https://github.com/modem-dev/hunk) most notably — shares that bet. rift's angle within it: structural diffs, review that goes deeper than viewing (tracking, test impact, staging, history), and composable output on everything.

| Capability                               | rift | [hunk](https://github.com/modem-dev/hunk) | [lazygit](https://github.com/jesseduffield/lazygit) | [forgit](https://github.com/wfxr/forgit) | [delta](https://github.com/dandavison/delta) | [difftastic](https://difftastic.wilfred.me.uk/) |
| ---------------------------------------- | :--: | :---------------------------------------: | :-------------------------------------------------: | :--------------------------------------: | :------------------------------------------: | :---------------------------------------------: |
| Structural (syntax-aware) diffs          |  ✅  |                    ❌                     |                         ❌                          |                    ❌                    |                      ❌                      |                       ✅                        |
| Interactive multi-file review            |  ✅  |                    ✅                     |                         ✅                          |                    ✅                    |                      ❌                      |                       ❌                        |
| Review tracking (content-keyed marks)    |  ✅  |                    ❌                     |                         ❌                          |                    ❌                    |                      ❌                      |                       ❌                        |
| Test-impact lens (`--tests`)             |  ✅  |                    ❌                     |                         ❌                          |                    ❌                    |                      ❌                      |                       ❌                        |
| Interactive hunk staging                 |  ✅  |                    ❌                     |                         ✅                          |                    ✅                    |                      ❌                      |                       ❌                        |
| Log & stash browsing                     |  ✅  |                    ❌                     |                         ✅                          |                    ✅                    |                      ❌                      |                       ❌                        |
| `--print` / `--json` on every command    |  ✅  |                    ❌                     |                         ❌                          |                    ❌                    |                      —                       |                       ❌                        |
| Watch mode (live reload)                 |  ✅  |                    ✅                     |                         ✅                          |                    ❌                    |                      —                       |                       ❌                        |
| Moved-line detection                     |  🔜  |                    ✅                     |                         ❌                          |                    ❌                    |                      ❌                      |                       ❌                        |
| Transient — invoke, act, return to shell |  ✅  |                    ✅                     |                         ❌                          |                    ✅                    |                      —                       |                        —                        |
| Single static binary                     |  ✅  |                    ❌                     |                         ✅                          |                    ❌                    |                      ✅                      |                       ✅                        |
| Inline agent annotations                 |  🔜  |                    ✅                     |                         ❌                          |                    ❌                    |                      ❌                      |                       ❌                        |
| Mouse support                            |  ✅  |                    ✅                     |                         ✅                          |                    ❌                    |                      —                       |                        —                        |

✅ yes · ❌ no · — not applicable · 🔜 planned. delta and difftastic are diff renderers/pagers rather than browsers, so several rows don't apply — rift uses difftastic as its diff engine and falls back to a built-in line diff without it.

## Key Features

### Structural diffs

Powered by difftastic: reformatting noise disappears and you see what actually changed at the expression level, with syntax highlighting and intra-line emphasis. When difftastic isn't on your `$PATH`, rift falls back to git's own diff (with word-level and whitespace highlighting) — toggle between the two engines with `e`. In commit-range and base-ref diffs, a renamed file lists as a single `old → new` entry showing only its real content delta, not a delete plus an add.

![A structural diff of a refactor](docs/images/diff.png)

### Split-pane browsing

`rift diff` and `rift log` are two-panel browsers: a fuzzy-filterable list on the left, a structural diff preview on the right. Navigate with vim keys, jump hunk-to-hunk, and keep your place with a scrollbar and a sticky file header. Colored file-type icons and dimmed directories make the list scan fast. In the log, press `⏎` to drill into a commit's files, or `c` / `r` to cherry-pick / revert the selected commit.

![rift log — commit explorer with a live diff preview](docs/images/log.png)

`rift stash` shares the same browser for stash entries and their diffs:

![rift stash — stash manager](docs/images/stash.png)

### Test lens

Add `--tests` to `rift diff` or `rift log` to read a change by the _tests_ it touches instead of its files — handy for reviewing agent-generated code. Each spec is marked by how the diff changed it: `+` added, `→` renamed, `~` modified (the `~` cases, where an assertion can be quietly weakened under an unchanged name, are the ones worth a close read). The preview scrolls to the selected test and names it in the border. Toggle live between the file view and the tests view with `t`; works on the working tree, `--staged`, or a commit range, and supports `--print` / `--json`. Tests are extracted across Go, JavaScript/TypeScript, Ruby, Python, and Rust — including `t.Run` subtests and table-driven cases.

![rift diff --tests — the tests a change added, renamed, or modified](docs/images/tests.gif)

### Review tracking

In `rift diff`, press `r` to mark the selected file **reviewed** (a `✓` replaces its status glyph) and `U` to show only the files you haven't reviewed yet — so you can walk a large change file by file and watch the list shrink as you tick each one off. A mark is keyed to the file's content, so it **resets the moment the file changes** (e.g. an agent edits it again) and re-applies on revert, making it easy to re-review only what's new. Marks persist per worktree (in `.git/rift/`, uncommitted); `--unreviewed` narrows `--print` / `--json` / `--name-only` to the not-yet-reviewed files.

![rift diff — marking changes reviewed and filtering to what's left](docs/images/reviewed.gif)

### Watch mode

`rift diff --watch` keeps the browser live while something else edits the working tree — typically an agent writing code in another pane. The file list, stats, and diffs refresh in place (your selection survives), the tests lens re-collects, and because reviewed marks are content-keyed they reset the moment a file changes again — so the unreviewed filter always shows exactly what still needs eyes. A `watching` tag in the header shows it's live. Works with any working-tree scope, including a base ref (`rift diff main --watch`) and the staged side — and toggles live with `w`, so you can turn it on the moment an agent starts working without losing your place (enabling catches up on anything that changed while it was off).

### Interactive staging

`rift stage` replaces `git add -p` with a two-panel TUI: a file list with structural diff preview and hunk-level staging, the active hunk clearly marked in the gutter.

![rift stage — interactive hunk staging](docs/images/stage.gif)

### Composable output

Every command is interactive by default but never traps you — each supports `--print` (plain selection) and `--json` (structured) modes:

```bash
rift log                              # interactive TUI
rift log --print                      # one commit hash per line
rift log --json                       # structured JSON
rift version --json                   # {"version":"v0.5.0"} — pin tooling in scripts

# pipe into anything
rift diff --print | xargs -r "$EDITOR"          # open every changed file
rift log --json | jq '.[] | select(.files_changed > 10)'
```

### Scoping to paths and history

`rift diff` and `rift log` both take a trailing `-- <path>...` to scope to specific files or directories, and both accept refs — `rift diff` takes one or two commits, `rift log` a ref or a commit range:

```bash
rift diff -- internal/git            # only the changes under internal/git
rift diff HEAD~3 HEAD -- README.md   # one file's changes across a commit range
rift log main..HEAD                  # commits on the current branch only
rift log --all                       # commits from every branch
rift log -n 50 -- cmd/               # the last 50 commits touching cmd/
```

### Shell completion

rift generates completion scripts for bash, zsh, and fish (via cobra):

```bash
source <(rift completion bash)                        # load for the current shell
rift completion zsh  > "${fpath[1]}/_rift"             # zsh
rift completion fish > ~/.config/fish/completions/rift.fish
```

Run `rift completion --help` for per-shell install instructions.

### Shared TUI

Every work screen (`diff`, `log`, `stash`, `stage`, and the tests lens) shares the same navigation and modal keys; each adds a few screen-specific actions. Press `?` on any screen for its live keybinding overlay.

**Shared keys**

| Key                         | Action                                                                       |
| --------------------------- | ---------------------------------------------------------------------------- |
| `j`/`k`, `↑`/`↓`            | move selection / scroll the preview                                          |
| `J`/`K`, `⇧↑`/`⇧↓`, `]`/`[` | step to the next / previous item (even while reading)                        |
| `gg`/`G`                    | jump to first / last                                                         |
| `{`/`}`                     | previous / next diff section (previous / next hunk in `stage`)               |
| `ctrl-d`/`ctrl-u`           | half-page down / up                                                          |
| `ctrl-f`/`ctrl-b`           | page down / up                                                               |
| `⇥`                         | switch between the list and preview panes                                    |
| `/`                         | fuzzy-filter (`⏎` accept, `esc` clear)                                       |
| `y`                         | yank the selection (path, commit hash, or stash ref)                         |
| `?`                         | keybinding overlay                                                           |
| `q`, `ctrl-c`               | quit                                                                         |
| `esc`                       | leave the current mode (filter, help, drilldown) — never quits a work screen |

**Per-screen keys**

| Screen     | Keys                                                                                                                                                        |
| ---------- | ----------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `diff`     | `s` toggle staged · `w` toggle watch · `t` tests lens · `\` diff layout · `e` engine · `o` open in `$EDITOR` · `r` mark reviewed · `U` show only unreviewed |
| `log`      | `⏎` drill into the commit's files (or tests) · `\` layout · `e` engine · `c` cherry-pick · `r` revert                                                       |
| `stash`    | `a` apply · `p` pop · `x` drop · `\` layout · `e` engine                                                                                                    |
| `stage`    | `s` stage · `u` unstage · `a` stage all · `o` open in `$EDITOR` · `e` engine (fixed layout, no `\`)                                                         |
| tests lens | `t` back to files · `s` toggle staged · `w` toggle watch · `\` layout · `e` engine · `o` open the test                                                      |

The `menu` launchpad is a plain list: `⏎` selects, `/` filters, and `esc`/`q` quit. Destructive actions (`stash` pop/drop, `log` cherry-pick/revert) confirm inline with `y`/`n`.

**Mouse**

The mouse works everywhere the keyboard does: the wheel scrolls the pane under the pointer (over a list it moves the selection), a click selects the row it lands on and focuses the pane it hit, and in the `menu` a click on the selected entry launches it. Drag across a diff preview to select lines — they highlight as you drag, and releasing copies them to the clipboard (ANSI-stripped, no borders or gutters; OSC 52 fallback over SSH). Set `RIFT_MOUSE=off` if you'd rather keep the terminal's native text selection — and note most terminals bypass mouse tracking with shift-click/drag either way.

### Environment

- `RIFT_THEME=light|dark` — force the color palette instead of detecting the terminal's background. Unset, rift asks the terminal and falls back to the dark palette when there's no reply.
- `RIFT_ICONS=ascii` (or `none`) — suppress the Nerd Font file-type icons for terminals without a patched font; the icon column drops out cleanly.
- `RIFT_MOUSE=off` — disable mouse tracking (wheel/click), keeping the terminal's native text selection.
- `NO_COLOR` — disable all ANSI color.
- `VISUAL` / `EDITOR` — the editor `o` opens (falling back to `vi`).

## Installation

On macOS or Linux with [Homebrew](https://brew.sh):

```bash
brew install madhermit/tap/rift
```

Or the install script, which downloads the right prebuilt binary for your platform and verifies its checksum against the release's `SHA256SUMS`:

```bash
curl -fsSL https://raw.githubusercontent.com/madhermit/rift/main/install.sh | bash
```

Prebuilt binaries for Linux and macOS (amd64 and arm64), plus the `SHA256SUMS` file, are attached to every [Release](https://github.com/madhermit/rift/releases) if you'd rather download and verify by hand.

> **Installing with `go install`?** `go install github.com/madhermit/rift@latest` works, but it builds _without_ the `grammar_subset*` tags that trim gotreesitter's embedded tree-sitter grammars — so the binary is much larger (~43 MB vs ~21 MB for a release build), and `rift version` reports the module version rather than a release tag. To match the release build, clone the repo and run `mise run install`, or build with the tags yourself:
>
> ```bash
> go build -tags "$GRAMMAR_TAGS" .   # GRAMMAR_TAGS as defined in mise.toml
> ```

### External Tools

On first run, rift automatically downloads [difftastic](https://difftastic.wilfred.me.uk/) to `~/.local/share/rift/bin/` if it's not already on your `$PATH`. If it's unavailable (offline, or the download fails), rift falls back to git's own diff with word-level highlighting — no external tools are required.

## Agent-Friendly

The `--json` output on every command gives agents structural understanding that raw git can't provide:

```bash
# inspect changes as structured data
rift diff --json | jq '.[] | select(.status == "Modified")'

# the tests a change added, renamed, or modified
rift diff --tests --json

# the files a human hasn't (re-)reviewed yet
rift diff --unreviewed --name-only

# list recent commits
rift log --json -n 10 | jq '.[].hash'
```

rift also ships a built-in [skill](internal/skill/SKILL.md) that teaches an agent the whole review workflow — inspecting a changeset, reading structural diffs, checking test impact, and respecting review state:

```bash
rift skill        # print it
rift skill path   # write it to a stable path and print the location
```

Tell your agent: _"Run `rift skill path` and follow that skill to review this change."_ Pair it with `rift diff --watch` in your own terminal and the review loop closes — you watch files tick off (and re-surface) as the agent works.

## Status

The core consumption commands — `diff`, `log`, `stash`, `stage` — are implemented and meant to be a daily driver (latest tag **v0.7.0**), with ongoing work on the diff/log reading experience: colored file icons, scrollbars, sticky file headers, and a structural fallback when difftastic is absent. Planned next: config, local code review, and inline agent annotations (file-based, in the same store as reviewed marks — no daemon).

Worktree and branch _management_ are intentionally out of scope — rift stays worktree-aware and pairs with [worktrunk](https://github.com/max-sixty/worktrunk).

See [CHANGELOG.md](CHANGELOG.md) for release notes.

## License

[MIT](LICENSE)
