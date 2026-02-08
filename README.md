# rift

Syntax-aware, worktree-native, composable fuzzy git tool.

rift wraps the git workflows where UX is the bottleneck — staging, diffing, branching, reviewing, conflict resolution — with structural understanding (via [difftastic](https://difftastic.wilfred.me.uk/) and [mergiraf](https://mergiraf.org/)), first-class worktree support, and composable output that works for both humans and scripts.

```
rift              # contextual launchpad
rift diff         # syntax-aware diff browser
rift add          # interactive staging with hunk/line granularity
rift log          # structural commit explorer
rift branch       # worktree-aware branch manager
rift stash        # stash manager
rift wt           # worktree manager
rift checkpoint   # named snapshots for iterative review
rift conflict     # mergiraf-powered conflict resolver
rift review       # risk-triaged structural code review
rift commit       # interactive conventional commits
rift bisect       # visual interactive bisect
rift pr           # forge integration (GitHub/GitLab/Gitea)
```

## Why

[forgit](https://github.com/wfxr/forgit) and [git-fuzzy](https://github.com/bigH/git-fuzzy) proved that fuzzy search over git objects is a massive UX win. But they're shell scripts piping strings through fzf — every diff is flat text, worktrees are invisible, and you can't pipe the output into anything.

[lazygit](https://github.com/jesseduber/lazygit) is the gold standard for git TUIs, but it's a resident app you live inside, with no composable output and line-based diffs.

rift occupies the space between them: **transient** (invoke, act, return to shell), **structural** (diffs understand your code's syntax), and **composable** (every command has `--print` and `--json` modes).

## Key Features

### Structural Diffs

Powered by difftastic. Reformatting noise disappears. You see what actually changed at the expression level, not what lines moved.

### Worktrees as First-Class Citizens

Every command is worktree-aware. `rift wt` manages worktrees with fuzzy search, dirty-state tracking across all worktrees, and cross-worktree structural diffs.

### Composable Output

Every command supports three output modes:

```bash
rift log                              # interactive TUI
rift log --print                      # one commit hash per line
rift log --json                       # structured JSON

# pipe into anything
rift branch --print | xargs git rebase
rift log --json | jq '.[] | select(.files_changed > 10)'
```

### Interactive Staging, Reimagined

`rift add` replaces `git add -p` with a three-panel TUI: file list, structural diff preview, and hunk/line/expression-level staging with bidirectional navigation.

### Risk-Triaged Code Review

`rift review` classifies changes by AST node type — signature changes and logic modifications surface first, formatting-only changes sink to the bottom. Review state persists across sessions.

### Syntax-Aware Conflict Resolution

`rift conflict` wraps mergiraf to show three-way structural diffs alongside auto-resolution suggestions. Accept, adjust, or pick a side — one keypress per conflict.

### Checkpoints

Named snapshots of your working state without committing. Diff between checkpoints, restore to a previous one, or promote a checkpoint to a real commit.

```bash
rift checkpoint "v1"              # snapshot current state
rift diff @checkpoint             # what changed since v1?
rift checkpoint restore "v1"      # roll back (non-destructive)
```

## Installation

### From Source

Requires Go 1.25+.

```bash
go install github.com/madhermit/rift@latest
```

### From Release

Download the binary for your platform from [Releases](https://github.com/madhermit/rift/releases) and place it on your `$PATH`.

### External Tools

On first run, rift will offer to download [difftastic](https://difftastic.wilfred.me.uk/) and [mergiraf](https://mergiraf.org/) to `~/.local/share/rift/bin/`. If they're already on your `$PATH`, those are used instead. If unavailable, rift falls back to built-in line diffs and standard conflict markers.

## Configuration

`~/.config/rift/config.toml` for global settings, `.rift.toml` in a repo root for per-project overrides.

```toml
[diff]
engine = "difftastic"       # or "line"

[merge]
engine = "mergiraf"
auto_accept = false

[worktree]
base_dir = "../{repo}-worktrees"

[commit]
conventional = true
```

## Agent-Friendly

rift doesn't manage AI agents — it's the tool agents reach for. The `--json` output on every command gives agents structural understanding that raw git can't provide:

```bash
# agent checks its own changes
rift diff --json | jq '[.[] | .changes[] | select(.risk == "high")]'

# agent verifies no unresolvable conflicts before requesting review
rift review --conflicts main feature-auth --json

# agent generates a review summary
rift review --summary --json
```

## Status

rift is under active development. See the [design document](git-flux-design.md) for the full architecture and phased roadmap.

## License

MIT
