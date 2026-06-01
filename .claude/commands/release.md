---
description: Cut a new rift release — roll the changelog, commit, tag, and hand off the push
argument-hint: [version like v0.2.0; omit to be prompted]
allowed-tools: Bash, Read, Edit, Write
---

You are cutting a release of **rift**. Target version: **$ARGUMENTS**

Releases are automated: pushing a `v*` tag triggers `.github/workflows/release.yml`, which runs tests, cross-builds the Linux/macOS amd64+arm64 binaries (with `Version=<tag>` baked in via ldflags), and publishes a GitHub Release with the binaries + auto-generated notes. **Your job is everything up to and including creating the tag.** This environment usually can't push, so you finish by handing the user the push commands — never push or create the GitHub release yourself.

## 0. Determine the version (if `$ARGUMENTS` is empty)

- Latest tag: `git tag --sort=-v:refname | head -1`.
- Review what's shipping: `git log <latest>..HEAD --oneline`.
- Propose a semver bump and **ask the user to confirm** before continuing. rift is pre-1.0, so: **patch** (`v0.1.x`) for fixes/polish, **minor** (`v0.x.0`) for a new or **removed** command/flag or a notable feature batch.

## 1. Preflight (abort on any failure)

- Working tree is clean: `git status --porcelain` must be empty. If not, stop and tell the user.
- On `main`: `git rev-parse --abbrev-ref HEAD` should be `main`.
- `mise run check` passes (fmt + lint + test).
- All four targets cross-compile: `mise run release <version>`, then `rm -rf dist` (it's a throwaway verification; `dist/` is gitignored).

## 2. Roll the changelog (`CHANGELOG.md`)

- Rename `## [Unreleased]` → `## [<x.y.z>] - <YYYY-MM-DD>` (today's date).
- Ensure that entry has the user-facing highlights grouped under **Added / Changed / Removed / Fixed**. If `[Unreleased]` was thin, fill it from `git log <latest>..HEAD` — describe *what users notice*, not commit-by-commit internals; skip pure refactors/test/tooling.
- Add a fresh empty `## [Unreleased]` heading above the new entry.
- Update the compare links at the bottom: point `[Unreleased]` at `compare/v<x.y.z>...HEAD`, and add `[<x.y.z>]: https://github.com/madhermit/rift/compare/<prev-tag>...v<x.y.z>`.

## 3. Bump the README

- Update the "latest tag **vX.Y.Z**" reference in the `## Status` section to the new version.

## 4. Commit, then tag

- Commit the release prep as a single commit: `Release v<x.y.z>`. **No `Co-Authored-By` trailer** (see CLAUDE.md).
- Create an annotated tag on that commit:
  `git tag -a v<x.y.z> -m "<short summary from the changelog highlights>"`.
  If you reference the editor env var in the message, write it as `\$EDITOR` so the shell doesn't expand it.
- Verify: `git describe --tags --exact-match HEAD` prints the new tag, and `git rev-parse v<x.y.z>^{commit}` equals `HEAD`.

## 5. Hand off the push

Print these for the user to run from a shell with push access, and explain the tag push is what triggers the release workflow:

```bash
git push origin main
git push origin v<x.y.z>
```

Then offer the changelog highlights, lightly reworded, as a blurb for the top of the GitHub Release body (the workflow's auto notes list the commits below it).

Do **not** push, create the GitHub release, or run `gh release` yourself.
