# git-flux: Design Document

## A syntax-aware, worktree-native, composable fuzzy git tool

---

## Thesis

forgit and git-fuzzy showed that wrapping git with fuzzy search dramatically improves the developer experience. But they're shell scripts piping strings through fzf. That ceiling limits them in three ways: every diff is a flat string with no structural understanding, worktrees are invisible, and the output is for humans only — you can't pipe it into anything.

git-flux is a single Go binary that treats **structural diffs** (via difftastic), **syntax-aware merging** (via mergiraf), **worktrees**, and **composable output** as first-class concerns. It occupies the space between "aliases with fzf" and "full TUI like lazygit" — fast, focused, transient, and deeply aware of your code's actual structure.

As AI coding agents become the dominant way code is produced, review is becoming the bottleneck. AI-authored PRs carry 1.7× more issues (GitClear, 2024), and the volume is exploding. git-flux doesn't try to be an agent orchestrator or compete with Claude Code and OpenCode — instead, it's a sharp, composable tool that both humans and agents reach for. A human gets a beautiful TUI with structural diffs and risk-aware review. An agent gets `--json` output with the same structural understanding. Same tool, two interfaces, no wrapper needed.

---

## Core Design Principles

1. **Structural, not textual.** Diffs understand syntax. Merges understand syntax. Staging understands syntax.
2. **Worktrees are first-class.** Every command is worktree-aware. Switching context means switching worktrees, not stashing and branching.
3. **Composable by default.** Every command has a `--print` mode that outputs structured data (JSON or plain selection) to stdout. Interactive mode is the default, but the tool never traps you.
4. **Transient, not resident.** Invoke, act, return to your shell. Not a persistent TUI you live inside.
5. **Single binary, minimal dependencies.** No fzf, no bash, no delta, no bat. One `brew install` or binary download. Difftastic and mergiraf are auto-installed on first run (downloaded to `~/.local/share/git-flux/bin/`) or detected on `$PATH` — the user never installs them separately. If download fails or the user is offline, git-flux falls back to built-in line diff with syntax highlighting. The git-flux binary itself stays lean (~10MB); the managed toolchain adds ~30MB on first use.
6. **Agent-friendly by being composable.** Every command's `--json` output is structured enough for an agent to reason over. git-flux doesn't manage agents — it's the tool agents (and humans) use.

---

## Non-Goals

These are explicitly out of scope. They're not future features — they reflect deliberate design boundaries.

- **git-flux is not a persistent TUI.** It doesn't try to replace lazygit. You invoke a command, do a thing, and return to your shell. If you want a resident application you live inside, use lazygit.
- **git-flux is not an agent orchestrator.** It doesn't spawn agents, manage task queues, or coordinate multi-agent workflows. It's the tool agents (and humans) call, not the thing that manages them.
- **git-flux does not aim for git parity.** It covers the ~12 commands that benefit most from structural awareness and fuzzy interactivity. `git push`, `git fetch`, `git remote` — these work fine already. git-flux wraps the workflows where UX is the bottleneck.
- **git-flux is not a merge tool replacement.** It provides a better frontend to merge conflict resolution (via mergiraf), but it doesn't try to replace your editor's merge mode or a dedicated 3-way merge tool like Meld.
- **git-flux is not a code review platform.** The `review` command is for local triage before you open a PR, not a replacement for GitHub's review interface, Reviewable, or Graphite.

---

## Architecture

### Binary & Invocation

Drop the binary as `git-flux` on your `$PATH`. Git automatically discovers it as a subcommand:

```
git flux              # interactive menu
```

The bare `git flux` command shows a status-first landing screen: current branch, dirty file count, worktree summary, and a fuzzy-searchable command list. Think of it as a contextual launchpad — not a dashboard you stare at, but a quick-orient-then-act entrypoint:

```
┌─────────────────────────────────────────────────────┐
│  main ✓  │  3 worktrees (1 dirty)  │  2 stashes     │
├─────────────────────────────────────────────────────┤
│  > diff         structural diff browser             │
│    add          interactive staging                  │
│    log          commit explorer                      │
│    review       risk-triaged code review             │
│    branch       branch manager                       │
│    wt           worktree manager                     │
│    checkpoint   named snapshots                      │
│    ...                                               │
│                                                      │
│  Type to filter · Enter to select · q to quit        │
└─────────────────────────────────────────────────────┘
```

```
git flux add          # interactive staging with hunk/line granularity
git flux log          # structural log explorer
git flux diff         # syntax-aware diff browser
git flux branch       # branch manager
git flux stash        # stash manager
git flux wt           # worktree manager (first-class)
git flux checkpoint   # named snapshots for iterative review (first-class)
git flux conflict     # merge conflict resolver (mergiraf-powered)
git flux bisect       # interactive bisect
git flux pr           # forge integration (GitHub/GitLab/Gitea)
git flux commit       # interactive commit with conventional commit support
git flux review       # code review with structural risk triage (first-class)
```

### Internal Stack

```
┌─────────────────────────────────────────────────┐
│                   TUI Layer                      │
│            bubbletea / lipgloss / bubbles        │
│         (fuzzy finder, split panes, modals)      │
├─────────────────────────────────────────────────┤
│                Structural Layer                  │
│   difftastic (diff)  │  mergiraf (merge/resolve) │
│        tree-sitter parsing for both              │
├─────────────────────────────────────────────────┤
│                  Git Layer                       │
│     go-git for reads  │  shelled git for writes  │
│        worktree registry & awareness             │
├─────────────────────────────────────────────────┤
│                Composable I/O                    │
│  --print [--format]  │  --json  │  stdin/stdout  │
│      pipe-friendly structured output             │
└─────────────────────────────────────────────────┘
```

**Why shell out to difftastic and mergiraf rather than reimplement?** See the dedicated sections below (Why Difftastic, Why Mergiraf). In short: both are complex, well-tested Rust tools. git-flux shells out and parses their structured output, becoming the interactive frontend they've never had.

**Distribution strategy for managed binaries:** On first run, git-flux checks for `difft` and `mergiraf` on `$PATH` at compatible versions. If not found, it downloads the correct platform binaries to `~/.local/share/git-flux/bin/` with a progress indicator and user confirmation. Subsequent runs version-check from cache and skip the download. This keeps the git-flux binary lean (~10MB), avoids cross-compiling Rust into the Go release pipeline, and still delivers a "one install" experience. If the network is unavailable, git-flux falls back to built-in line diff and standard conflict markers — it never blocks on a failed download.

**Why bubbletea?** It's the de facto Go TUI framework (Charm ecosystem). Unlike fzf piping, it supports real split panes, modal dialogs, resizable panels, and concurrent rendering. The fuzzy finder is just one widget among many.

---

## First-Class Concern: Worktrees

### The Problem

Git worktrees are the correct answer to "I need to context-switch but don't want to stash." Yet they have zero interactive tooling. You end up memorizing paths and running `git worktree list` constantly.

### The Design

`git flux wt` is the worktree command, but worktree awareness permeates everything:

```
git flux wt                    # list all worktrees with fuzzy search
git flux wt new <branch>       # create worktree + branch in one step
git flux wt switch             # fuzzy-pick a worktree and cd into it
git flux wt remove             # fuzzy-pick and remove (with dirty check)
git flux wt remove <wt> [<wt>] # remove one or more worktrees by name
git flux wt status             # show dirty state across ALL worktrees
git flux wt diff               # diff across worktrees (structural)
```

#### Worktree-Aware Status Bar

Every git-flux command shows a subtle status line:

```
 worktree: ~/src/project-main (main)  │  3 others: feature-auth ● feature-api  deploy ✓
```

The `●` means dirty, `✓` means clean. At a glance, you know the state of every worktree.

#### Worktree-Aware Branching

`git flux branch` shows which branches are checked out in which worktrees:

```
  main              ~/src/project-main
  feature-auth ●    ~/src/project-auth
  feature-api       ~/src/project-api
  bugfix-123        (not checked out)
```

Trying to check out a branch that's active in another worktree offers to switch you there instead.

#### Shell Integration for `cd`

Because `cd` can't be run from a subprocess, `git flux wt switch` works via one of:

- **Eval mode:** `eval "$(git flux wt switch --print-cd)"` — outputs `cd /path/to/worktree`
- **Shell function:** Ship a tiny shell wrapper that sources from the binary:
  ```bash
  gfw() { cd "$(git flux wt switch --print)" }
  ```
- **tmux/terminal integration:** Option to open a new tmux pane/window at the worktree path

#### Cross-Worktree Operations

```
git flux wt diff           # compare the same file across two worktrees (structural)
```

Structural diff between working trees — including uncommitted changes — is something plain git doesn't do well. `git flux wt diff` lets you compare the live state of a file across two worktrees using difftastic.

Note: `git cherry-pick` already works across worktrees (the commit graph is shared), so git-flux doesn't wrap it. If you want to interactively pick a commit from another worktree's branch, use `git flux log --print` scoped to that branch and pipe it to `git cherry-pick`.

---

## First-Class Concern: Composable I/O

### The Problem

forgit and git-fuzzy are interactive-only. Their output goes to the screen or into a git command. You can't use them as building blocks in scripts or pipelines.

### The Design

Every git-flux command supports three output modes:

| Flag | Behavior |
|------|----------|
| (default) | Interactive TUI — fuzzy search, preview, act |
| `--print` | Output the selected item(s) to stdout, one per line |
| `--json` | Output structured JSON to stdout |

`--format` is a modifier on `--print` that applies a format string instead of the default plain output. It is not a separate mode:

```bash
git flux log --print                          # plain: one commit hash per line
git flux log --print --format '{hash}\t{message}'  # formatted plain output
git flux log --json                           # full structured JSON
```

This means git-flux is simultaneously a TUI tool and a scripting primitive:

```bash
# Interactive: opens TUI, you pick a branch, it checks out
git flux branch

# Composable: pick a branch interactively, use it in a script
BRANCH=$(git flux branch --print)
git rebase "$BRANCH"

# Fully scripted: get all branches with metadata as JSON
git flux branch --json | jq '.[] | select(.ahead > 5)'

# Pipe into itself
git flux log --print | xargs git revert

# Multi-select + pipe
git flux log --print --multi | xargs git cherry-pick
```

#### Structured JSON Schema

Each command defines a schema. For example, `git flux log --json`:

```json
[
  {
    "hash": "a1b2c3d",
    "author": "Jane Doe <jane@example.com>",
    "date": "2025-02-01T14:30:00Z",
    "message": "feat: add OAuth support",
    "files_changed": 12,
    "insertions": 340,
    "deletions": 87,
    "conventional": {
      "type": "feat",
      "scope": null,
      "description": "add OAuth support"
    }
  }
]
```

This makes git-flux the backbone of CI scripts, custom dashboards, and automation — not just a human-facing tool.

---

## First-Class Concern: Interactive Staging (`add -p` Reimagined)

### The Problem

`git add -p` is one of the most useful git features and one of the worst user experiences. The single-character prompts (`y/n/q/a/d/s/e`), the inability to go back, the lack of any visual context, and the fact that hunk splitting is a recursive nightmare — it all adds up to people either staging whole files or avoiding interactive staging entirely.

### The Design

`git flux add` presents a three-panel TUI:

```
┌─────────────────┬─────────────────────────────────────────┐
│ FILES            │ DIFF (structural via difftastic)         │
│                  │                                         │
│ ● src/auth.go   │   fn validate_token(token: &str) {      │
│   src/config.go  │ -     if token.len() < 8 {              │
│   src/main.go    │ +     if token.len() < 16 {             │
│   tests/auth.go  │          return Err("too short")        │
│                  │      }                                  │
│                  │ +     if !token.starts_with("sk_") {    │
│                  │ +         return Err("invalid prefix")  │
│                  │ +     }                                  │
│                  │      Ok(())                             │
│                  │  }                                      │
│                  │                                         │
├─────────────────┴─────────────────────────────────────────┤
│ [s]tage hunk  [l]ine  [f]ile  [u]nstage  [Enter] next    │
│ [/] search    [tab] toggle panel    [q]uit  [c]ommit      │
└───────────────────────────────────────────────────────────┘
```

#### Key Innovations

**Structural diff in the preview.** The right panel uses difftastic, so you see what actually changed syntactically, not what lines moved. If someone reformatted the file, you see the real semantic changes, not 200 lines of whitespace noise. This is transformative for staging — you can confidently stage the hunks that matter.

**Hunk, line, and expression granularity.** Three levels of staging:
- `s` — stage the entire hunk (like `git add -p` → `y`)
- `l` — enter line-select mode: use arrow keys or visual selection to pick individual lines
- `x` — expression-level: because we have the AST (from tree-sitter, same as difftastic), we can offer expression-level granularity. Stage an entire function, a single argument change, or one branch of an if-else.

**Bidirectional navigation.** Unlike `git add -p`, you can go back. Arrow up/down through hunks, jump between files, undo a staging decision. The state isn't committed until you press `c` or `q`.

**Fuzzy search across files and hunks.** Press `/` and type — filter the file list by path, or filter hunks by content. "Show me all hunks that touch `validate`" is now trivial.

**Integrated commit flow.** Press `c` from the staging view and you're in the commit editor without leaving the TUI. If conventional commits are configured, you get an interactive type/scope/description flow.

**Unstage with symmetry.** `git flux add` and `git flux unstage` share the same interface. `u` in the staging view unstages a hunk. The entire staging/unstaging workflow is one cohesive experience.

---

## First-Class Concern: Difftastic Integration

### Why Difftastic?

Difftastic is a structural diff — it parses code with tree-sitter, compares syntax trees, and shows what actually changed at the expression level. Reimplementing its Dijkstra-based graph diffing algorithm in Go would be a multi-year effort with worse results. Instead, git-flux shells out to `difft` and parses its structured JSON output. Difftastic is designed to be composed (as `GIT_EXTERNAL_DIFF`); git-flux provides the interactive layer it's never had.

Structural diffs are superior to line-based diffs in multiple ways:

- **Reformatting noise disappears.** If code is rewrapped over multiple lines but the logic is unchanged, difftastic shows nothing changed.
- **Wrapped expressions are matched correctly.** Adding a wrapper function around existing code shows just the wrapper, not the entire inner expression as changed.
- **Real line numbers.** No cryptic `@@ -5,6 +5,7 @@` headers.
- **Language-aware.** Knows that `x-1` is three tokens in JavaScript but one in Lisp.

### Integration Points

Difftastic powers the diff display everywhere in git-flux:

| Command | How difftastic is used |
|---------|----------------------|
| `git flux diff` | Primary diff browser — structural diffs with fuzzy file selection |
| `git flux add` | Preview panel shows structural diff for each file/hunk |
| `git flux log` | Commit detail view shows structural diff of the commit |
| `git flux stash` | Stash preview shows structural diff against current |
| `git flux pr` | PR diff browser uses structural diffs |
| `git flux bisect` | Shows structural diff between good/bad candidates |
| `git flux conflict` | Shows structural three-way diff of conflict sides |
| `git flux wt diff` | Cross-worktree structural comparison |

### Invocation Strategy

Difftastic is invoked as an external process (`difft`). git-flux manages its availability:

1. **System:** If `difft` is on `$PATH` and version >= 0.60, use it.
2. **Managed:** If not found, auto-install to `~/.local/share/git-flux/bin/` on first run (with user confirmation). Subsequent runs use the cached binary.
3. **Fallback:** If neither is available (offline, download failed), fall back to built-in line diff with chroma syntax highlighting. Never break.

The `--display=json` flag (or parsing difftastic's structured output) gives git-flux the data it needs to render diffs in its own TUI rather than just piping terminal output.

### Structural Diff in the Fuzzy Finder

Unlike forgit (which shows `git diff` output in the fzf preview), git-flux can correlate structural diff hunks with AST nodes. This enables:

- **Filtering by change type:** "Show me only hunks where function signatures changed"
- **Grouping by scope:** Hunks grouped by function/class/module, not by file position
- **Semantic hunk labels:** Instead of `@@ -42,7 +42,9 @@`, show `fn validate_token() → modified`

---

## First-Class Concern: Mergiraf Integration

### Why Mergiraf?

Mergiraf is a syntax-aware merge driver — it uses tree-sitter to parse conflicting versions, matches AST nodes with the GumTree algorithm, then resolves conflicts at the expression level rather than line level. Reimplementing GumTree + fact-based merging in Go would be another multi-year effort. Like difftastic, mergiraf is designed to be composed (as a git merge driver); git-flux shells out and wraps it in an interactive conflict resolution TUI. It supports 33+ languages and can resolve conflicts that git's `ort` strategy cannot:

- **Independent additions to the same block** (e.g., two people add different imports)
- **Move + edit** (one side moves a function, the other edits it)
- **Commutative parents** (order-independent collections like struct fields, imports)
- **Granular conflicts** (pinpoints the exact expression that conflicts, not the whole block)

On the Linux kernel history, mergiraf resolved ~6% of conflicts that ort couldn't (per mergiraf's published benchmarks).

### Integration Points

#### `git flux conflict` — Interactive Conflict Resolution

After a failed merge/rebase/cherry-pick, `git flux conflict` presents:

```
┌─────────────────┬─────────────────────────────────────────┐
│ CONFLICTED FILES │ THREE-WAY DIFF                          │
│                  │                                         │
│ ● src/auth.go   │  BASE           OURS          THEIRS    │
│   src/config.yml │  fn validate   fn validate   fn valid.. │
│                  │    len < 8       len < 16      len < 12 │
│                  │                  prefix check            │
│                  │                                         │
│                  │  ─── mergiraf suggestion ───            │
│                  │  fn validate_token(token: &str) {       │
│                  │      if token.len() < 16 {  ← OURS     │
│                  │          ...                            │
│                  │      }                                  │
│                  │      if !token.starts_with("sk_") {     │
│                  │          ...               ← OURS       │
│                  │      }                                  │
│                  │      Ok(())                             │
│                  │  }                                      │
├─────────────────┴─────────────────────────────────────────┤
│ [a]ccept mergiraf  [o]urs  [t]heirs  [b]ase  [e]dit      │
│ [r]eview changes   [n]ext file   [q]uit                   │
└───────────────────────────────────────────────────────────┘
```

**The workflow:**

1. git-flux detects conflicted files after a merge/rebase/cherry-pick.
2. For each file, it runs `mergiraf solve` to attempt automatic resolution.
3. The TUI shows the three-way structural diff (using difftastic for each pair) alongside mergiraf's proposed resolution.
4. The user can accept mergiraf's resolution, pick a side, or edit manually.
5. `r` triggers `mergiraf review` — showing exactly what mergiraf changed and why.
6. Resolved files are automatically staged.

**This is the killer workflow.** Today, encountering a merge conflict means: read the markers, open a merge tool or editor, manually reconstruct intent, test, stage. With git-flux, it's: see the structural diff, review mergiraf's suggestion (which is correct most of the time for clean conflicts), accept or adjust, move on.

#### Proactive Merge Conflict Preview

Before you even merge:

```bash
git flux merge-preview feature-branch
```

Runs a dry merge, identifies potential conflicts, and shows you what mergiraf can auto-resolve vs. what you'll need to handle. This turns merge anxiety into merge planning.

#### Convenience: Install Mergiraf as Merge Driver

git-flux can set up mergiraf as git's merge driver on your behalf — it doesn't become the merge driver itself, it just writes the configuration:

```bash
git flux config --install-merge-driver
```

This writes the appropriate entries to `.gitattributes` and `.gitconfig` to route supported languages through mergiraf, with fallback to ort for unsupported ones. It's equivalent to manually configuring mergiraf per its documentation, but saves you from looking up the syntax. This is a Phase 4 convenience feature, not core functionality.

---

## First-Class Concern: Code Review with Structural Risk Triage

### The Problem

Review is becoming the bottleneck — whether the code was written by a human, an AI agent, or both. GitHub's 2025 Octoverse reports ~41% of new code is now AI-assisted. Research suggests AI-authored PRs carry 1.7× more issues than human-authored ones (GitClear, 2024), and the issues aren't surface-level — they're subtle logic errors, missed edge cases, and context-blind architectural decisions.

Every existing review tool treats diffs as flat text. Nobody has built a terminal review tool that understands that:

1. Not all changes deserve equal attention. Boilerplate needs a glance; a changed validation function needs focus.
2. Structural diffs make review faster. Seeing "this function's signature changed and these 14 call sites were updated" is faster than scanning 200 lines of line-diff.
3. Review state should persist. If you reviewed 3 of 7 files, you should be able to close the terminal and pick up where you left off.
4. The same review data that helps a human should be available to a script or agent via `--json`.

### The Design

`git flux review` is a first-class command, not an alias for `git flux diff`.

```
git flux review                     # review current branch against main
git flux review feature-auth        # review a specific branch
git flux review --agent             # review all pending agent worktrees
git flux review --risk high         # filter to high-risk changes only
```

#### Semantic Change Categories

Because git-flux has the AST (via tree-sitter/difftastic), it can classify every change:

| Category | Risk | Example |
|----------|------|---------|
| **Signature change** | 🔴 High | Function parameters, return types, public API |
| **Logic modification** | 🔴 High | Conditionals, loops, error handling, validation |
| **New code** | 🟡 Medium | New functions, new files, new modules |
| **Call site update** | 🟡 Medium | Existing code updated to match a signature change |
| **Dependency change** | 🟡 Medium | Import additions, go.mod/package.json changes |
| **Boilerplate / scaffold** | 🟢 Low | Generated tests, config files, repetitive patterns |
| **Formatting / refactor** | 🟢 Low | Renamed variables, reformatted code (structural diff: no semantic change) |
| **Documentation** | 🟢 Low | Comments, README, docstrings |

The review TUI groups changes by category and sorts by risk:

```
┌─────────────────────┬──────────────────────────────────────────┐
│ REVIEW QUEUE         │ STRUCTURAL DIFF                          │
│                      │                                          │
│ 🔴 HIGH RISK (3)     │  fn validate_token(                      │
│  ├ auth.go:validate  │ -    token: &str                         │
│  ├ auth.go:refresh   │ +    token: &str, scope: &Scope          │
│  └ db.go:query       │  ) -> Result<Claims> {                   │
│                      │ +    if !scope.allows(token) {           │
│ 🟡 MEDIUM (5)        │ +        return Err(AuthError::Scope)    │
│  ├ handler.go:new    │ +    }                                   │
│  ├ routes.go:update  │      // existing validation...           │
│  ├ config.go:add     │  }                                      │
│  ├ go.mod:dep        │                                          │
│  └ types.go:struct   │  ─── agent: claude-code (wt: auth) ───  │
│                      │  task: "Add scope-based auth"            │
│ 🟢 LOW (12)          │                                          │
│  ├ *_test.go (8)     │                                          │
│  ├ README.md         │                                          │
│  └ formatting (3)    │                                          │
│                      │                                          │
│ ✅ Reviewed: 4/20    │                                          │
├──────────────────────┴──────────────────────────────────────────┤
│ [a]pprove hunk  [!] flag issue  [s]kip  [n]ext category        │
│ [c]omment  [/] search  [R] reviewed  [q]uit                    │
└─────────────────────────────────────────────────────────────────┘
```

#### Attention Budgeting

When reviewing agent output, the problem isn't finding issues — it's knowing where to *look*. git-flux uses the structural diff to compute an attention budget:

```bash
git flux review --summary
```

Output:
```
Branch: feature-auth (agent: claude-code, worktree: ~/src/project-auth)
Task: "Add scope-based authorization to token validation"

Total changes: 847 lines across 20 files
  🔴 High-risk:    89 lines (3 files)  — estimated review: 8 min
  🟡 Medium-risk:  234 lines (5 files) — estimated review: 12 min
  🟢 Low-risk:     524 lines (12 files) — estimated review: 3 min (skim)

Structural summary:
  • 2 function signatures changed (validate_token, refresh_token)
  • 1 new type added (Scope)
  • 14 call sites updated to pass new Scope parameter
  • 8 test files added (all new)
  • 3 files reformatted only (no semantic change)

Suggested review order: auth.go → db.go → types.go → routes.go → (skim tests)
```

This alone saves 20 minutes per review. Instead of opening a diff and scrolling, you know exactly where to focus and approximately how long it'll take.

#### Review Checkpoints (Persistent State)

Review state persists in `.git/flux/reviews/`:

```bash
git flux review feature-auth       # review, approve 5 files, quit
# ... later ...
git flux review feature-auth       # picks up where you left off — 5 already ✅
```

Each hunk can be marked: ✅ approved, ❌ rejected, 💬 commented, ⏭ skipped. This state survives across sessions and terminal closures. It lives in `.git/flux/reviews/`, which is local to the clone. To share review state across machines, git-flux can optionally push it to hidden refs (`refs/flux/reviews/*`), similar to `git notes`.

#### Multi-Branch Conflict Preview

Before merging any branch:

```bash
git flux review --conflicts feature-auth feature-api
```

This does a dry-merge of the specified branches against each other and against main, using mergiraf to identify which conflicts are auto-resolvable and which need human attention:

```
Merge preview:

  feature-auth → main             ✅ clean merge
  feature-api  → main             ✅ clean merge
  feature-auth ↔ feature-api     ⚠️  2 conflicts in routes.go
    • mergiraf auto-resolves: 1 (independent additions to router)
    • needs human review: 1 (both modify errorHandler signature)

Suggested merge order: feature-auth first, then feature-api (fewer conflicts)
```

With `--json`, an agent or CI script gets the same data as structured output and can act on it.

---

## First-Class Concern: Checkpoints for Iterative Review

### The Problem

When working with AI agents (or just iterating on code), you often want to:

1. Make changes, review them, mark them as "known good"
2. Make more changes (refactor, extend, fix)
3. See exactly what changed since the known-good state
4. Repeat: checkpoint, iterate, diff, checkpoint, iterate, diff

Git's staging area is the closest primitive, but it's a single unnamed slot. Commits work but pollute history. Stash removes your working changes. None of them support structural diffs or named reference points you can easily diff between.

### The Design

`git flux checkpoint` creates named snapshots of your working state that you can diff against, without committing or stashing.

```bash
# Create a checkpoint of current state (staged + unstaged)
git flux checkpoint "initial auth implementation"

# Agent makes more changes...

# What changed since the checkpoint?
git flux diff @checkpoint/initial-auth-implementation

# Shorthand: diff against most recent checkpoint
git flux diff @checkpoint

# List all checkpoints
git flux checkpoint list

# Diff between two checkpoints
git flux diff @checkpoint/v1..@checkpoint/v2

# Delete a checkpoint
git flux checkpoint drop initial-auth-implementation

# Clear all checkpoints
git flux checkpoint clear
```

**Shorthand alias:** `@checkpoint` can be abbreviated to `@cp` everywhere — e.g., `git flux diff @cp`, `git flux diff @cp/v1..@cp/v2`. Both forms are always valid.

#### Under the Hood: Shadow Commits

Checkpoints are real git commits stored on hidden refs (`refs/flux/checkpoints/<name>`). This means:

- All of git's diffing machinery works
- Difftastic structural diffs work
- They capture both staged and unstaged changes
- Space-efficient (git dedupes objects)
- Don't appear in `git log`, don't pollute history
- Survive across sessions (they're in `.git/`)

Implementation:
```bash
# "git flux checkpoint foo" does:
# 1. Snapshot ALL working state (staged + unstaged) into a temp index
GIT_INDEX_FILE=.git/flux-tmp-index git add -A
TREE=$(GIT_INDEX_FILE=.git/flux-tmp-index git write-tree)
rm .git/flux-tmp-index
# 2. Create a commit object pointing to that tree
COMMIT=$(git commit-tree $TREE -p HEAD -m "checkpoint: foo")
# 3. Store it on a hidden ref
git update-ref refs/flux/checkpoints/foo $COMMIT

# "git flux diff @checkpoint/foo" does:
difft <(git show refs/flux/checkpoints/foo:path) <(cat path)  # per-file structural diff
```

Note: Because checkpoints use a temporary index, the real staging area and working tree are never disturbed.

#### Checkpoint Workflows

**Basic iterative review:**
```bash
# Agent implements feature
git flux diff                              # review changes vs main
git flux checkpoint "v1"                   # looks good, save it

# Ask agent to refactor
git flux diff @checkpoint                  # what changed since v1?
git flux checkpoint "v2-refactored"        # save this state too

# Ask agent to add error handling  
git flux diff @checkpoint                  # changes since v2
git flux diff @checkpoint/v1..@checkpoint  # cumulative: v1 to now
```

**Checkpoint-aware staging:**
```bash
# Stage only changes since last checkpoint
git flux add --since-checkpoint

# Opens staging TUI filtered to just the delta since checkpoint
# Useful when you've already reviewed v1 and just want to stage the new stuff
```

**Checkpoint restore (like stash pop, but non-destructive):**
```bash
# Current state is broken, roll back to checkpoint
git flux checkpoint restore "v1"

# Working tree now matches v1 checkpoint
# The checkpoint still exists (unlike stash pop)
# Your broken state is auto-checkpointed as "pre-restore" in case you need it
```

**Checkpoint promote (turn scratch work into real commits):**
```bash
# You've been iterating, now v3 is ready
git flux checkpoint promote "v3" -m "feat(auth): implement token validation"

# Creates a real commit on current branch
# Optionally squashes working changes into it
```

**Diff checkpoints in the review TUI:**
```bash
git flux review --since-checkpoint

# Review TUI shows only changes since last checkpoint
# Risk scoring applies to the delta, not the whole branch
```

#### Checkpoint Metadata

Each checkpoint stores:
```
refs/flux/checkpoints/v1
├── tree (the actual file state)
├── message ("initial auth implementation")
├── timestamp
├── parent-checkpoint (optional, for checkpoint chains)
└── base-commit (what HEAD was when checkpoint was created)
```

This enables:
```bash
git flux checkpoint list --verbose

NAME                      CREATED          BASE      FILES  DESCRIPTION
v3-with-tests            2 min ago        a1b2c3d   +12    "added test coverage"
v2-refactored            18 min ago       a1b2c3d   +8     "cleaner structure"
v1-initial               45 min ago       a1b2c3d   +5     "initial auth implementation"

# Visual timeline
git flux checkpoint timeline

main (a1b2c3d)
  └─ v1-initial (+5 files)
       └─ v2-refactored (+3 files from v1)
            └─ v3-with-tests (+4 files from v2)
                 └─ (working tree: +2 files from v3)
```

#### Why Not Just Use Stash?

Stash is close, but:

| | `git stash` | `git flux checkpoint` |
|---|---|---|
| Removes working changes | Yes (must pop) | No |
| Named | Sort of (`-m`) | Yes, first-class |
| Easy to diff between | No | Yes (`@cp/a..@cp/b`) |
| Structural diff | No | Yes (difftastic) |
| Visual timeline | No | Yes |
| Restore without destroying | No | Yes |
| Filtered staging | No | Yes (`add --since-checkpoint`) |

Stash is designed for "save my work, switch context, come back later." Checkpoints are designed for "mark this as a reference point while I keep working."

#### JSON Output for Agents

```bash
git flux checkpoint list --json
```
```json
[
  {
    "name": "v2-refactored",
    "ref": "refs/flux/checkpoints/v2-refactored",
    "created": "2025-02-05T14:30:00Z",
    "base_commit": "a1b2c3d",
    "description": "cleaner structure",
    "files_changed": 8,
    "parent_checkpoint": "v1-initial"
  }
]
```

```bash
git flux diff @checkpoint/v1..@checkpoint/v2 --json
```
```json
{
  "from": "v1-initial",
  "to": "v2-refactored", 
  "changes": [
    {
      "file": "src/auth.go",
      "type": "refactor",
      "risk": "low",
      "description": "Extracted validation into separate function"
    }
  ]
}
```

An agent can use this to understand its own iteration history, verify that a refactor didn't change semantics (only `refactor` and `formatting` change types), or restore to a known-good checkpoint if something went wrong.

---

## How Agents Consume git-flux (Without Special Agent Features)

### The Insight

git-flux doesn't need agent orchestration or MCP to be agent-friendly. The composable I/O design — `--print`, `--json`, `--format` — already makes it the best git interface for agents. An agent running in Claude Code, OpenCode, Cursor, or any terminal-based coding tool can shell out to git-flux and get structural, risk-scored data that no other tool provides.

The philosophy: **be the sharpest tool in the shed, not the shed itself.**

### What Agents Get from `--json`

Every command emits structured data when asked. The key differentiator is that this isn't just reformatted `git` output — it includes structural analysis that only git-flux can provide.

**`git flux diff --json`** — Structural diff with change classification:
```json
[
  {
    "file": "src/auth.go",
    "changes": [
      {
        "type": "signature_change",
        "risk": "high",
        "function": "validate_token",
        "before": "func validate_token(token string) error",
        "after": "func validate_token(token string, scope Scope) error",
        "affected_call_sites": 14
      },
      {
        "type": "new_code",
        "risk": "medium",
        "function": "Scope.allows",
        "lines_added": 12
      }
    ]
  },
  {
    "file": "src/handler.go",
    "changes": [
      {
        "type": "call_site_update",
        "risk": "low",
        "function": "handleLogin",
        "description": "Updated to pass scope parameter"
      }
    ]
  }
]
```

An agent reading this can reason about whether all call sites were updated, whether new code has tests, whether the changes are internally consistent — without parsing line diffs.

**`git flux review --summary --json`** — Risk-scored review summary:
```json
{
  "branch": "feature-auth",
  "total_changes": { "lines": 847, "files": 20 },
  "risk_breakdown": {
    "high":   { "lines": 89,  "files": 3,  "estimated_minutes": 8 },
    "medium": { "lines": 234, "files": 5,  "estimated_minutes": 12 },
    "low":    { "lines": 524, "files": 12, "estimated_minutes": 3 }
  },
  "structural_summary": [
    "2 function signatures changed (validate_token, refresh_token)",
    "1 new type added (Scope)",
    "14 call sites updated to pass new Scope parameter",
    "8 test files added",
    "3 files: formatting only (no semantic change)"
  ],
  "suggested_review_order": ["auth.go", "db.go", "types.go", "routes.go"]
}
```

**`git flux wt --json`** — Worktree state across the whole project:
```json
[
  {
    "path": "~/src/project-main",
    "branch": "main",
    "dirty": false,
    "ahead": 0, "behind": 0
  },
  {
    "path": "~/src/project-auth",
    "branch": "feature-auth",
    "dirty": true,
    "ahead": 7, "behind": 0,
    "dirty_files": ["src/auth.go", "src/handler.go"]
  }
]
```

**`git flux conflict --json`** — Merge conflict analysis with mergiraf resolution info:
```json
{
  "file": "routes.go",
  "conflicts": [
    {
      "auto_resolved": true,
      "strategy": "commutative_parent",
      "description": "Independent additions to router.Group()"
    },
    {
      "auto_resolved": false,
      "description": "Both sides modify errorHandler signature",
      "ours": "func errorHandler(w http.ResponseWriter, err error)",
      "theirs": "func errorHandler(ctx context.Context, err error)"
    }
  ]
}
```

### Example: Agent Workflow Using git-flux as a Tool

An agent (Claude Code, Codex, etc.) working in a worktree can use git-flux without any special integration:

```bash
# Agent checks what it changed (structural, not line noise)
git flux diff --json

# Agent verifies its changes are internally consistent
git flux diff --json | jq '[.[] | .changes[] | select(.type == "signature_change")] | length'
# → 2 signatures changed

git flux diff --json | jq '[.[] | .changes[] | select(.type == "call_site_update")] | length'  
# → 14 call sites updated (matches expectation)

# Agent checks for merge conflicts before asking human to review
git flux review --conflicts main feature-auth --json | jq '.[] | select(.auto_resolved == false)'
# → 0 unresolvable conflicts, safe to request review

# Agent generates a review summary for the human
git flux review --summary
# → Structured output the agent can include in a PR description or Slack message

# Human reviews with the TUI
git flux review feature-auth
# → Risk-triaged, structural diffs, persistent state
```

The agent doesn't need MCP, doesn't need a special protocol. It shells out to `git flux` with `--json` and gets structural understanding that raw `git` can't provide. This is the Unix philosophy: do one thing well, expose it cleanly.

#### Agent Instruction Files

For teams using AI coding agents, `git flux agent-docs` (Phase 4) will generate a snippet suitable for `CLAUDE.md`, `AGENTS.md`, or `.cursorrules` that teaches the agent to prefer `git flux diff --json` over `git diff`, use `git flux review --summary --json` for self-checks before requesting human review, and leverage checkpoints for iterative work. Until then, the README and `--help` output are sufficient for agents that can read documentation.

---

## End-to-End Example: Reviewing Parallel Worktree Work

```bash
# You've been running Claude Code in three worktrees. Time to review.

# Quick overview: what's the state of everything?
git flux wt status
# → main (clean), feature-auth (+847 lines, 20 files), 
#   feature-api (+189 lines, 8 files), bugfix-429 (+12 lines, 2 files)

# Start with the smallest. Quick review:
git flux review bugfix-429
# → 12 lines, all low-medium risk. Approve in 2 minutes. Merge.

# The big one. Check the risk budget first:
git flux review --summary feature-auth
# → 89 lines high-risk (3 files), 234 medium, 524 low
# → Suggested order: auth.go → db.go → types.go → routes.go

# Review, focusing on high-risk:
git flux review feature-auth
# → TUI opens sorted by risk. Review 3 high-risk files (8 min).
# → Skim medium. Skip formatting-only files.
# → Flag one issue in auth.go. Close terminal.

# Come back later, fix was pushed. Re-review:
git flux review feature-auth
# → Picks up where you left off. 1 new hunk addresses the flag. Approve.

# Before merging both features, check for conflicts:
git flux review --conflicts feature-auth feature-api
# → 1 conflict in routes.go, mergiraf auto-resolves it. 

# Merge:
git merge feature-auth && git merge feature-api
git flux wt remove feature-auth feature-api bugfix-429
```

Total human review time: ~15 minutes for ~1000 lines across three branches. The structural risk triage and persistent review state are what compress the time — you're never reading boilerplate or re-reviewing files you already approved.

---

## End-to-End Example: Iterative Agent Refinement with Checkpoints

This workflow shows how checkpoints enable iterative review when asking an agent to refine code in multiple passes.

```bash
# Ask Claude Code to implement a feature
> "Add OAuth token validation with scope checking"

# Agent writes code. Review the initial implementation:
git flux diff
# → 5 files changed, structural diff shows new types and functions

# Looks good as a starting point. Checkpoint it.
git flux checkpoint "v1-initial-implementation"

# Ask for a refactor
> "Extract the validation logic into a separate package"

# Agent refactors. Now see ONLY what changed since v1:
git flux diff @checkpoint
# → Shows just the refactoring changes, not the whole feature
# → Structural diff: "moved validate_token to pkg/auth, 0 logic changes"

# The refactor looks clean. Checkpoint it.
git flux checkpoint "v2-extracted-package"

# Ask for error handling improvements
> "Add detailed error types instead of generic errors"

# See what changed since v2:
git flux diff @checkpoint
# → New error types, updated return signatures

# Hmm, the agent changed more than expected. Compare against original:
git flux diff @checkpoint/v1-initial-implementation
# → Shows cumulative changes: refactor + error handling

# See the full timeline:
git flux checkpoint timeline
# main (a1b2c3d)
#   └─ v1-initial-implementation (+5 files)
#        └─ v2-extracted-package (+2 files from v1)
#             └─ (working tree: +4 files from v2)

# The error handling approach is wrong. Roll back to v2:
git flux checkpoint restore v2-extracted-package
# → Working tree now matches v2
# → Broken state auto-saved as "pre-restore-<timestamp>"

# Try again with better instructions
> "Add error types, but keep the same function signatures"

# Check the new attempt:
git flux diff @checkpoint
# → Better. Only internal changes, signatures preserved.

git flux checkpoint "v3-error-handling"

# Review the full feature (all changes since main):
git flux review
# → Risk-triaged view of everything

# Now selectively stage. Only stage changes since v2 (the new error handling):
git flux add --since-checkpoint v2-extracted-package
# → Staging TUI shows only the delta between v2 and now

# Or stage everything and commit:
git add -A
git commit -m "feat(auth): add OAuth token validation with scope checking"

# Clean up checkpoints:
git flux checkpoint clear
```

The checkpoint workflow transforms agent iteration from "hope the refactor didn't break anything" to verifiable incremental changes. At each step you see exactly what the agent modified, and you can roll back without losing history.

---

## Secondary Features

### Interactive Bisect

```
git flux bisect
```

A visual bisect experience:

- Shows the remaining search space as a visual graph
- Each step shows the structural diff between the test commit and known-good
- Press `g` (good) or `b` (bad) to advance
- Optionally run a test command and auto-advance
- `--print` outputs the final bad commit hash

### Forge Integration (PR Browser)

```
git flux pr                    # list & fuzzy-search PRs
git flux pr view               # open PR detail with structural diff
git flux pr checkout            # check out a PR in a new worktree (!)
```

The worktree integration here is key: `git flux pr checkout` creates a worktree for the PR branch, so you can review it without disrupting your current work. When you're done, `git flux wt remove` cleans it up.

Abstracts over GitHub (via `gh` API), GitLab, and Gitea. Auto-detects the forge from the remote URL.

### Commit with Conventional Commits

```
git flux commit
```

If configured, presents an interactive conventional commit flow:

1. Select type: `feat`, `fix`, `chore`, `refactor`, `docs`, `test`, `ci`
2. Select scope (auto-suggested from changed file paths): `auth`, `config`, `api`
3. Write description
4. Optional body and footer (breaking changes, issue references)

Produces: `feat(auth): add OAuth token validation`

### Session History & Undo

git-flux records actions in `~/.local/share/git-flux/sessions/`:

```bash
git flux undo                  # walk back through session actions
git flux history               # view session log
```

Unlike git-fuzzy's snapshots (state-based), this is action-aware:

```
14:32:01  staged src/auth.go (3 hunks)
14:32:15  staged src/config.go (1 hunk)
14:32:20  unstaged src/auth.go hunk #2
14:33:01  committed "feat(auth): add validation"
```

---

## Configuration

`~/.config/git-flux/config.toml`:

```toml
[core]
# TUI theme: "auto" detects dark/light from terminal
theme = "auto"

# Fuzzy matching algorithm (rarely needs changing)
# "smith-waterman" = better match quality (default)
# "fzf-v2" = fzf-compatible scoring for users who prefer familiar ranking
fuzzy_algorithm = "smith-waterman"

[diff]
# Structural diff engine
engine = "difftastic"              # or "line" for classic line diff
# Fallback when difftastic is unavailable or for unsupported languages
fallback = "chroma"                # built-in syntax-highlighted line diff

[merge]
# Use mergiraf for conflict resolution
engine = "mergiraf"
# Auto-accept mergiraf resolutions (or always prompt for review)
auto_accept = false
# Compact conflict markers
compact = true

[worktree]
# Default base directory for new worktrees
base_dir = "../{repo}-worktrees"
# Auto-create worktree for PR checkouts
pr_worktree = true

[commit]
# Conventional commits
conventional = true
# Allowed types
types = ["feat", "fix", "chore", "refactor", "docs", "test", "ci", "perf"]
# Auto-suggest scope from file paths
auto_scope = true

[compose]
# Default output format for --print
format = "plain"                   # or "json"
# Delimiter for multi-select --print output
delimiter = "\n"

[keybindings]
# Override any keybinding
stage_hunk = "s"
stage_line = "l"
stage_expression = "x"
accept_merge = "a"
next_file = "n"
search = "/"

# Per-repo overrides via .git-flux.toml in repo root
```

Per-repository overrides via `.git-flux.toml` in the repo root (tracked or in `.git/`).

---

## Prior Art

git-flux builds on and learns from several tools:

**forgit** — Proved that fuzzy-finding git objects (branches, logs, stashes) with fzf is a massive UX win. Limitation: shell-only, no structural understanding, no composable output. Every command is an alias that pipes through fzf and feeds back into git. git-flux takes the "fuzzy search everything" insight and rebuilds it with a real TUI and structural awareness.

**git-fuzzy** — Extended forgit's approach with richer preview panes and a more cohesive command structure. Limitation: still shell scripts on fzf, still line-based diffs, still interactive-only. Demonstrated that developers want more than a fuzzy picker — they want preview, context, and action in one flow.

**lazygit** — The gold standard for git TUIs. Rich, persistent, well-designed, huge community. Limitation: it's a resident application (you live inside it), has no composable output, and uses line-based diffs. git-flux is explicitly not competing with lazygit's "live in the TUI" model — it occupies the transient/composable quadrant that lazygit leaves empty.

**difftastic** — A breakthrough in diff quality. Structural, syntax-aware, language-aware diffs that eliminate formatting noise and show what actually changed. Limitation: it's a diff display tool with no interactive layer. git-flux provides the interactive frontend — staging, review, filtering — that difftastic's output deserves.

**mergiraf** — Syntax-aware merge driver that resolves conflicts git's ort strategy can't. Limitation: it's a merge driver, not a user-facing tool. You configure it in `.gitattributes` and it runs silently. git-flux wraps mergiraf in an interactive conflict resolution TUI with three-way structural diffs and one-key accept/reject.

**delta** — Beautiful terminal diff renderer with syntax highlighting. Limitation: it's a pager, not a tool. It makes line diffs look better but doesn't change what they show. git-flux chose difftastic over delta because structural understanding matters more than rendering — and git-flux handles its own rendering via bubbletea.

---

## Competitive Positioning

```
                      structural awareness
                             ↑
                             │
                  git-flux ★ │
                             │
         forgit              │              lazygit
         git-fuzzy           │
                             │
  ──────────────────────────────────────────→ full TUI
  transient/composable                      resident/app

                             │
                             │
                             │
                     plain git│ + aliases
                             │
```

|  | forgit | git-fuzzy | lazygit | **git-flux** |
|---|---|---|---|---|
| Diff quality | git diff | git diff | git diff (+ delta integration) | **difftastic (structural)** |
| Merge conflicts | manual | manual | side-by-side with pick ours/theirs | **mergiraf (AST-aware auto-resolve)** |
| Worktrees | none | none | basic list + switch | **first-class: status, switch, cross-WT ops** |
| Checkpoints | none | none | none | **named snapshots, timeline, incremental diff** |
| Composability | none | none | none | **--print, --json, --format** |
| Code review | none | none | none | **risk-triaged, persistent, structural** |
| `add -p` | delegates to git | basic staging | hunk + line staging | **hunk + line + expression staging** |
| Install | plugin manager | clone + PATH | binary | **single binary** |
| Dependencies | fzf, bash, delta | fzf, bash | none | **none (auto-installs difft, mergiraf)** |
| Invocation style | aliases (ga, glo) | menu + commands | persistent TUI | **transient: invoke, act, exit** |

---

## Performance Considerations

git-flux's identity is "transient, not resident" — which means latency on every invocation matters. Difftastic is slower than `git diff` (typically 2–10× depending on file size and language), and mergiraf must parse files with tree-sitter before it can reason about them. The risk classification in `review` requires AST analysis of every changed file.

The strategy:

- **Lazy evaluation.** Don't parse or diff anything until the user (or `--json` output) actually requests it. In the TUI, only the currently visible file is diffed; others are parsed in the background.
- **Caching.** Difftastic output is cached by blob hash pair in `.git/flux/cache/`. A cached structural diff for `(blob_a, blob_b)` is valid forever (content-addressed). This means re-opening a review or re-running a diff is instant after the first pass.
- **Timeout and fallback.** If difftastic takes longer than a configurable threshold (default: 3s per file), fall back to the built-in line diff with syntax highlighting for that file and continue. Never block the TUI.
- **Parallel diffing.** When `--json` is used (non-interactive), diff all files in parallel up to `GOMAXPROCS`. The TUI can prefetch the next N files while the user reads the current one.
- **Large file cutoff.** Files above a configurable size (default: 50,000 lines) skip structural diff entirely and use line diff. Difftastic's Dijkstra-based algorithm is O(n × m) in the worst case; this keeps pathological cases bounded.

Target latency: `git flux diff` on a typical PR (~20 files, ~500 lines changed) should feel instant (<200ms to first render, with structural diffs streaming in).

---

## Risk Classification: How It Works

The review section's risk categories aren't hand-waved heuristics — they're derived from AST diffing. Here's the actual approach:

**Step 1: Parse both versions with tree-sitter.** Difftastic already does this. git-flux consumes difftastic's structured JSON output, which includes matched AST node pairs and change types.

**Step 2: Classify changed AST nodes by kind.** Tree-sitter node types map to risk categories:

| AST Node Type | Classification | Risk |
|--------------|----------------|------|
| `function_declaration`, `method_definition` (signature portion: name, params, return type) | Signature change | 🔴 High |
| `if_statement`, `for_statement`, `match_expression`, `try_statement`, `return_statement` | Logic modification | 🔴 High |
| New `function_declaration`, new file | New code | 🟡 Medium |
| `call_expression` where callee matches a changed signature | Call site update | 🟡 Medium |
| `import_declaration`, dependency files (`go.mod`, `package.json`) | Dependency change | 🟡 Medium |
| Node changed but parent expression tree is semantically equivalent (difftastic reports "no change" at expression level) | Formatting / refactor | 🟢 Low |
| `comment`, `string` in docstring position | Documentation | 🟢 Low |

**Step 3: Language-specific overrides.** The classification is language-aware via tree-sitter grammars. In Python, a parameter change in a `def` without type annotations is still a signature change — we match on the `parameters` node, not on type syntax. In Go, a changed `error` return is always 🔴. These overrides are maintained as a small config layer per grammar.

**Step 4: Confidence and fallback.** When tree-sitter doesn't have a grammar for a language (or parsing fails), risk scoring degrades gracefully: all changes in that file are marked 🟡 Medium by default, and the file is flagged as "unclassified — no grammar available." The tool never makes a confident claim it can't back up.

**Limitations:** This approach works well for single-function changes but is weaker at cross-function reasoning (e.g., "this refactor moved logic from function A to function B, so the net semantic change is zero"). That kind of analysis would require whole-program understanding. For now, both the deletion in A and the addition in B are classified independently. This is a known limitation documented in `git flux review --help`.

---

## Error Handling Philosophy

git-flux follows a "degrade gracefully, never crash, always explain" approach:

- **Difftastic unavailable or unsupported language:** Fall back to built-in line diff with chroma syntax highlighting. Show a subtle indicator in the TUI (`[line diff — no grammar for .xyz]`) so the user knows they're not seeing structural output.
- **Mergiraf fails or produces incorrect resolution:** Never auto-apply mergiraf resolutions by default (`auto_accept = false` in config). When mergiraf errors, fall back to showing standard conflict markers and log the error for debugging. The user always has the option to pick ours/theirs/base or edit manually.
- **Checkpoint name collision:** Refuse and prompt. `git flux checkpoint "v1"` when `v1` exists → `"Checkpoint 'v1' already exists. Use --force to overwrite, or choose a different name."`
- **`wt switch` to a dirty worktree:** Warn and confirm. Show the dirty file count and ask whether to proceed, stash first, or abort.
- **Terminal capability issues:** Detect color support (`TERM`, `COLORTERM`, `NO_COLOR`). Fall back from true color → 256 color → 16 color → no color. TUI layout degrades from split panes → single pane on narrow terminals (<80 cols).
- **Pipe detection:** When stdout is not a TTY (i.e., git-flux is being piped), automatically behave as if `--print` was passed. Never render TUI escape codes into a pipe.

---

## Accessibility

- **Colorblind safety:** Risk indicators use shape/symbol in addition to color: `▲ HIGH`, `● MEDIUM`, `◆ LOW` alongside 🔴🟡🟢. The `--no-color` flag and `NO_COLOR` env var are respected throughout.
- **Keyboard-only navigation:** All TUI interactions are keyboard-driven by design. No mouse-only affordances. Tab order is logical (file list → diff pane → action bar).
- **Screen readers:** TUI frameworks (bubbletea) have limited screen reader support. For screen reader users, `--print` and `--json` modes provide equivalent access to all data. This is a known gap in the interactive mode; improvements will track upstream bubbletea accessibility work.
- **High-contrast themes:** The `theme = "high-contrast"` option uses bold text weight and maximum-contrast foreground/background pairs instead of relying on mid-tone syntax colors.

---

**Phase 1 — Foundation (MVP)**
- Single binary, `git flux` menu
- Managed auto-install of difftastic and mergiraf (download on first run, detect on `$PATH`)
- `diff` with difftastic integration
- `add` with hunk/line staging + structural preview
- `log`, `branch`, `stash` with fuzzy search
- `--print` and `--json` on all commands
- Basic config via TOML

**Phase 2 — Worktrees & Checkpoints**
- `wt` command suite (list, new, switch, remove, status)
- Cross-worktree structural diff (`wt diff`)
- Worktree awareness in branch, log, status bar
- `checkpoint` with shadow commits on hidden refs
- Checkpoint diffing (`diff @checkpoint`, `diff @cp/a..@cp/b`)
- Checkpoint restore and timeline
- Shell integration helpers for `cd`

**Phase 3 — Review & Merging**
- `review` with semantic change categories and risk scoring
- Review checkpoints (persistent approval state)
- `review --since-checkpoint` (filtered review)
- `add --since-checkpoint` (filtered staging)
- Attention budget summaries
- `conflict` with mergiraf integration
- `merge-preview` / `review --conflicts`

**Phase 4 — Power Features & Ecosystem**
- Expression-level staging (tree-sitter AST)
- `bisect` interactive mode
- `commit` with conventional commits
- `checkpoint promote` (turn checkpoint into commit)
- Session history and undo
- `pr` with GitHub/GitLab/Gitea abstraction
- PR checkout → worktree integration
- Homebrew, AUR, Nix, Scoop packaging
- Shell completions (bash, zsh, fish)
- Per-repo config overrides
- `config --install-merge-driver` convenience helper
- `git flux agent-docs` — outputs a recommended agent instruction snippet (`CLAUDE.md`, `AGENTS.md`, `.cursorrules`) for the current repo, teaching agents to use `git flux --json` instead of raw git

---

## Future Considerations

The following are recorded for future exploration, not V1 scope. They become interesting once the composable foundation is solid and real usage patterns emerge.

**Agent orchestration (`git flux agent`).** A dedicated subcommand for spawning agent worktrees with task metadata, monitoring progress across a dashboard, and managing the review→merge lifecycle. This would include `.git-flux-agent.toml` files for provenance, agent-aware status bars, and pre-flight merge checks. Worth building once the parallel-agent-worktree pattern stabilizes and we understand what orchestration agents actually need vs. what they handle themselves.

**MCP server mode (`git flux mcp`).** Exposing git-flux as a Model Context Protocol server (stdio + HTTP transport) so agents can call tools like `flux_diff`, `flux_review_state`, and `flux_conflict_check` via structured protocol instead of shelling out. This is compelling but the composable `--json` CLI already serves most agent needs. MCP becomes valuable if agents need live subscriptions (e.g., "notify me when review state changes") or if the ecosystem converges on MCP as the standard agent↔tool protocol.

**Agent provenance in git history.** Embedding agent metadata in commit trailers (`Agent-Task:`, `Agent-Session:`, `Spawned-By:`) to create a permanent, searchable record of which code was agent-authored. Enables queries like `git flux log --agent --risk high`. Interesting for auditability but requires community conventions around trailer formats.

**Review-as-gate for CI.** Exporting review state (`git flux review --state --json`) for consumption by CI pipelines, so a merge can be blocked until all high-risk hunks are human-approved. Bridges the gap between local review and PR-level gating.
