# rift

Syntax-aware, composable fuzzy git tool.

rift wraps the git workflows where UX is the bottleneck — staging, diffing, branching, stashing — with structural understanding via [difftastic](https://difftastic.wilfred.me.uk/) and composable output that works for both humans and scripts.

```
rift              # contextual launchpad
rift diff         # syntax-aware diff browser
rift stage        # interactive staging with hunk granularity
rift log          # structural commit explorer
rift branch       # fuzzy branch switcher
rift stash        # stash manager with diff preview
```

## Why

[forgit](https://github.com/wfxr/forgit) and [git-fuzzy](https://github.com/bigH/git-fuzzy) proved that fuzzy search over git objects is a massive UX win. But they're shell scripts piping strings through fzf — every diff is flat text, and you can't pipe the output into anything.

[lazygit](https://github.com/jesseduber/lazygit) is the gold standard for git TUIs, but it's a resident app you live inside, with no composable output and line-based diffs.

rift occupies the space between them: **transient** (invoke, act, return to shell), **structural** (diffs understand your code's syntax), and **composable** (every command has `--print` and `--json` modes).

## Key Features

### Structural Diffs

Powered by difftastic. Reformatting noise disappears. You see what actually changed at the expression level, not what lines moved.

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

### Interactive Staging

`rift stage` replaces `git add -p` with a two-panel TUI: file list with structural diff preview and hunk-level staging.

## Installation

```bash
curl -fsSL https://raw.githubusercontent.com/madhermit/rift/main/install.sh | bash
```

Or with Go 1.25+:

```bash
go install github.com/madhermit/rift@latest
```

Pre-built binaries for Linux and macOS are available on the [Releases](https://github.com/madhermit/rift/releases) page.

### External Tools

On first run, rift automatically downloads [difftastic](https://difftastic.wilfred.me.uk/) to `~/.local/share/rift/bin/` if it's not already on your `$PATH`. If the download fails, rift falls back to built-in line diffs — no external tools are required.

## Agent-Friendly

The `--json` output on every command gives agents structural understanding that raw git can't provide:

```bash
# inspect changes as structured data
rift diff --json | jq '.[] | select(.status == "modified")'

# list recent commits
rift log --json -n 10 | jq '.[].hash'
```

## Status

**v0.1.0** — core commands implemented (diff, log, branch, stash, stage). Worktree management, config, and code review planned for v0.2.0.

## License

[MIT](LICENSE)
