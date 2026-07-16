package diffui

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/madhermit/rift/internal/diff"
	"github.com/madhermit/rift/internal/git"
	"github.com/madhermit/rift/internal/review"
	"github.com/madhermit/rift/internal/tui"
)

var reviewedGlyph = lipgloss.NewStyle().Foreground(tui.Green)

// FilesChangedMsg tells the lens its changed-file set was re-listed externally
// (the watch poller): Files is the new set, already pathspec-filtered and
// sorted, and Hashes its worktree content hashes (computed off the event loop
// by the poll; nil means compute them here). The lens applies the set in
// place, keeping the selection and — for files whose content didn't change —
// the cached previews and the reader's scroll position.
type FilesChangedMsg struct {
	Files  []git.ChangedFile
	Hashes map[string]string
}

// reviewMarks is the reviewed-state lookup the row renderer reads: the persisted
// marks plus each file's current blob hash. Both are mutated in place — Toggle on
// the store, a fresh hash map after an edit — so the row closure that captures
// the *reviewMarks always reflects the current state.
type reviewMarks struct {
	state  *review.Reviewed
	hashes map[string]string
}

func (rm *reviewMarks) reviewed(path string) bool {
	return rm != nil && rm.state.IsReviewed(path, rm.hashes[path])
}

type Model struct {
	repo    *git.Repo
	engines tui.EngineToggle
	list    tui.SplitList[git.ChangedFile]

	staged     bool
	base       string
	target     string
	commitDiff bool
	display    diff.Display
	stream     *tui.PreviewStream // active "all changes" stream; nil otherwise

	files          []git.ChangedFile // the full changed set; the list shows a (filtered) view of it
	paths          []string          // pathspec scope (from `-- path...`), re-applied on reload
	marks          *reviewMarks      // nil for a commit diff — historical files don't get reviewed
	unreviewedOnly bool
	watching       bool // show the live-watch indicator in the header context
}

// prependAllEntry adds a synthetic "All" row (carrying the totals) that previews
// every file at once.
func prependAllEntry(files []git.ChangedFile) []git.ChangedFile {
	all := git.ChangedFile{Path: "", Status: "All"}
	for _, f := range files {
		all.Added += f.Added
		all.Deleted += f.Deleted
	}
	out := make([]git.ChangedFile, 0, len(files)+1)
	return append(append(out, all), files...)
}

// rowWithStat right-aligns stat (e.g. "+3 -1") at the end of a row of width w,
// dropping it if there's no room.
func rowWithStat(left, stat string, w int) string {
	if stat == "" {
		return left
	}
	gap := w - lipgloss.Width(left) - lipgloss.Width(stat)
	if gap < 1 {
		return left
	}
	return left + strings.Repeat(" ", gap) + stat
}

// pathRow renders prefix + a path (dim dir, emphasized basename) truncated to
// fit, right-aligning stat within total width w (reserving a one-column gap).
func pathRow(prefix string, selected bool, path, stat string, w int) string {
	avail := w - lipgloss.Width(prefix)
	if stat != "" {
		avail -= lipgloss.Width(stat) + 1
	}
	if avail < 1 {
		avail = 1
	}
	return rowWithStat(prefix+tui.RenderPath(path, avail, selected), stat, w)
}

func New(repo *git.Repo, engine diff.Engine, files []git.ChangedFile, staged bool, base, target string, lensToggle bool, paths []string, unreviewed bool) Model {
	commitDiff := target != ""
	engines := tui.NewEngineToggle(engine)

	var marks *reviewMarks
	if !commitDiff {
		marks = &reviewMarks{
			state:  review.LoadReviewed(repo),
			hashes: review.ContentHashes(repo, files),
		}
	}

	m := Model{
		repo:       repo,
		engines:    engines,
		staged:     staged,
		base:       base,
		target:     target,
		commitDiff: commitDiff,
		files:      files,
		paths:      paths,
		marks:      marks,
		// --unreviewed opens with the same filter the `U` key toggles live; a
		// commit diff has no review state, so the flag is meaningless there.
		unreviewedOnly: unreviewed && !commitDiff,
	}

	hints := [][2]string{{"/", "filter"}, {"⇥", "read"}}
	if lensToggle {
		hints = append(hints, [2]string{"t", "tests"})
	}
	if !commitDiff {
		hints = append(hints, [2]string{"s", "staged"}, [2]string{"w", "watch"})
	}
	hints = append(hints, [2]string{"\\", "layout"})
	if engines.CanToggle() {
		hints = append(hints, [2]string{"e", "engine"})
	}
	if !commitDiff {
		// In a commit diff the files are historical: editing the working-tree copy
		// (`o`) or reviewing it would be misleading, so those are working-tree only.
		hints = append(hints, [2]string{"o", "open"}, [2]string{"r", "review"}, [2]string{"U", "unreviewed"})
	}
	hints = append(hints, [2]string{"y", "yank"}, [2]string{"?", "help"}, [2]string{"q", "quit"})

	cfg := tui.SplitConfig[git.ChangedFile]{
		Screen:      "diff",
		ListTitle:   m.listTitle(),
		Branch:      repo.CurrentBranch(),
		Context:     tui.ContextLabel(target, engine.Name(), false),
		NavFraction: 30,
		EmptyStatus: m.emptyStatus(),
		Hints:       hints,
		// Match on both sides of a rename, so a moved file is findable by its old
		// name too (OldPath is empty for everything else).
		Match: func(f git.ChangedFile) string { return f.Path + " " + f.OldPath },
		PreviewTitle: func(f git.ChangedFile) string {
			if f.Path == "" {
				return "all changes"
			}
			return f.DisplayPath()
		},
		// Cache per file; the All entry depends on the filtered set, so skip it.
		// Staged/display changes clear the cache (SetItems / ClearCacheAndReload).
		CacheKey: func(f git.ChangedFile) string { return f.Path },
		Yank:     func(f git.ChangedFile) string { return f.Path },
		Row:      fileRow(marks),
	}

	m.list = tui.NewSplitList(cfg, m.displayFiles())
	return m
}

// fileRow renders a file row: a status glyph (or ✓ when reviewed), file-type
// icon, path, and +/- stat. The synthetic "All" entry renders its own banner.
func fileRow(marks *reviewMarks) func(git.ChangedFile, int, bool) string {
	return func(f git.ChangedFile, w int, selected bool) string {
		if f.Path == "" {
			return rowWithStat(tui.TextStyle(selected).Render("∗ All changes"), tui.DiffStat(f.Added, f.Deleted), w)
		}
		glyph := tui.StatusStyle(f.Status).Render(git.StatusChar(f.Status))
		if marks.reviewed(f.Path) {
			glyph = reviewedGlyph.Render("\u2713") // standard check (U+2713), font-independent
		}
		prefix := glyph + " " + tui.IconField(f.Path)
		return pathRow(prefix, selected, f.DisplayPath(), tui.DiffStat(f.Added, f.Deleted), w)
	}
}

// displayFiles is the list the SplitList shows. In the default view it's every
// changed file under a synthetic "All" entry. In the unreviewed-only view it's
// just the not-yet-reviewed files with no "All" row — so as you tick files off,
// the cursor (reset to the top by the re-filter) lands on the next one to review.
func (m Model) displayFiles() []git.ChangedFile {
	if m.unreviewedOnly && m.marks != nil {
		return m.marks.state.Unreviewed(m.files, m.marks.hashes)
	}
	if len(m.files) == 0 {
		return nil // no synthetic "All changes" row over an empty set — show the status
	}
	return prependAllEntry(m.files)
}

// SetWatching sets the "watching" tag in the header context, marking the lens
// as live-reloading. The flag persists on the model because the engine toggle
// rebuilds the label.
func (m Model) SetWatching(on bool) Model {
	m.watching = on
	m.list = m.list.SetContext(m.contextLabel())
	return m
}

func (m Model) contextLabel() string {
	return tui.ContextLabel(m.target, m.engines.Name(), m.watching)
}

// SetEmptyStatus overrides the footer text shown when the list is empty (e.g. a
// commit whose diff failed to load, distinct from "No changes found").
func (m Model) SetEmptyStatus(s string) Model {
	m.list = m.list.SetEmptyStatus(s)
	return m
}

// SetBreadcrumb marks this view as an embedded drilldown: it sets the header
// breadcrumb (e.g. "log ❯ a1b2c3d") and adds an "esc back" footer hint. Used by
// the log drilldown so the header names its origin and the way back shows.
func (m Model) SetBreadcrumb(screen string) Model {
	m.list = m.list.WithBreadcrumb(screen)
	return m
}

// SetFlash sets the embedded list's transient footer message (see
// SplitList.SetFlash); the lens uses it to show "collecting tests…".
func (m Model) SetFlash(s string) Model {
	m.list = m.list.SetFlash(s)
	return m
}

// listTitle is the list-pane title: the staged/unstaged toggle (or "changes" for
// a commit diff), plus the reviewed count — but only once you're actually
// reviewing (some file marked, or the unreviewed-only filter on), so it doesn't
// nag "0/N reviewed" at someone who never touches the feature.
func (m Model) listTitle() string {
	title := "changes"
	if !m.commitDiff {
		title = tui.ToggleTitle("unstaged", "staged", m.staged)
	}
	if m.marks == nil {
		return title
	}
	if n, total := m.marks.state.Count(m.marks.hashes), len(m.marks.hashes); total > 0 && (n > 0 || m.unreviewedOnly) {
		title += fmt.Sprintf(" · %d/%d reviewed", n, total)
	}
	if m.unreviewedOnly {
		title += " · unreviewed"
	}
	return title
}

func (m Model) emptyStatus() string {
	if m.unreviewedOnly {
		return "All changes reviewed"
	}
	return "No changes found"
}

func (m Model) Init() tea.Cmd { return nil }

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tui.SelectionChangedMsg:
		m.stream.Cancel()
		m.stream = nil
		if msg.CacheHit {
			return m, nil // preview served from cache; no new load
		}
		return m, m.previewCmd(msg.ReqID)
	case tui.StreamReadyMsg:
		var cmd tea.Cmd
		m.list, m.stream, cmd = tui.ApplyStream(m.list, m.stream, msg)
		return m, cmd
	case tui.ChunkMsg:
		var cmd tea.Cmd
		m.list, m.stream, cmd = tui.AdvanceStream(m.list, m.stream, msg)
		return m, cmd
	case tui.EditorClosedMsg:
		// The edit may have changed the file's diff (even reverting it entirely), and
		// can flip its reviewed state — so reload the file list (recomputing hashes),
		// preserving the selection, rather than leaving stale rows and +/- stats.
		return m.reloadFiles()
	case FilesChangedMsg:
		if m.commitDiff {
			return m, nil // historical set; nothing external can change it
		}
		return m.applyFiles(msg.Files, msg.Hashes)
	case tea.KeyPressMsg:
		if m.list.Filtering() || m.list.ShowingHelp() {
			break
		}
		switch msg.String() {
		case "\\":
			m.display = m.display.Next()
			return m.reloadFresh()
		case "e":
			if !m.engines.CanToggle() {
				break // only one engine available; nothing to toggle
			}
			m.engines = m.engines.Toggle()
			m.list = m.list.SetContext(m.contextLabel())
			return m.reloadFresh()
		case "o":
			// Disabled for a commit diff: the files are historical, so editing the
			// working tree copy would be misleading.
			if sel, ok := m.list.Selected(); ok && !m.commitDiff && sel.Path != "" {
				line := diff.FirstHunkLine(m.repo.Root(), m.staged, sel.Path)
				return m, tui.OpenInEditor(m.repo.Root(), sel.Path, line)
			}
		case "r":
			// `r` and `U` are not viewport/list scroll keys, so when reviewing is
			// off (a commit drilldown) they fall through and do nothing rather than
			// shadowing a binding. (space/u stay as the viewport's page-scroll keys.)
			if m.marks != nil {
				return m.toggleReviewed()
			}
		case "U":
			if m.marks != nil {
				return m.toggleUnreviewedOnly()
			}
		}
	}
	var cmd tea.Cmd
	m.list, cmd = m.list.Update(msg)
	return m, cmd
}

// toggleReviewed flips the selected file's reviewed mark. In the unreviewed-only
// view the file then drops out of the list, advancing to the next one.
func (m Model) toggleReviewed() (tea.Model, tea.Cmd) {
	sel, ok := m.list.Selected()
	if m.marks == nil || !ok || sel.Path == "" {
		return m, nil
	}
	m.marks.state.Toggle(sel.Path, m.marks.hashes[sel.Path])
	m.list = m.list.SetListTitle(m.listTitle())
	if m.unreviewedOnly {
		var cmd tea.Cmd
		m.list, cmd = m.list.SetItems(m.displayFiles())
		return m, cmd
	}
	return m, nil
}

// toggleUnreviewedOnly switches between showing all changed files and only the
// ones not yet reviewed.
func (m Model) toggleUnreviewedOnly() (tea.Model, tea.Cmd) {
	if m.marks == nil {
		return m, nil
	}
	m.unreviewedOnly = !m.unreviewedOnly
	m.list = m.list.SetListTitle(m.listTitle()).SetEmptyStatus(m.emptyStatus())
	var cmd tea.Cmd
	m.list, cmd = m.list.SetItems(m.displayFiles())
	return m, cmd
}

// reloadFresh drops the preview cache and reloads the current selection, after a
// setting (layout/engine) that affects rendering has changed.
func (m Model) reloadFresh() (tea.Model, tea.Cmd) {
	var cmd tea.Cmd
	m.list, cmd = m.list.ClearCacheAndReload()
	return m, cmd
}

// reloadFiles re-reads the changed-file list for the current scope (staged
// side, base ref, and pathspec) and applies it. A commit diff shows a fixed
// historical set, so it only reloads the current preview.
func (m Model) reloadFiles() (tea.Model, tea.Cmd) {
	if m.commitDiff {
		return m.reloadFresh()
	}
	files, err := m.repo.ListChanged(m.staged, m.base, "", m.paths...)
	if err != nil {
		return m, nil
	}
	return m.applyFiles(files, nil)
}

// applyFiles replaces the changed-file set (hashes may carry precomputed
// worktree content hashes; nil means compute them) and repopulates the list,
// keeping the selection on the same file when it survives. Only files whose
// record or content actually changed lose their cached preview — so a watch
// refresh triggered by some other file leaves the diff being read untouched.
func (m Model) applyFiles(files []git.ChangedFile, hashes map[string]string) (tea.Model, tea.Cmd) {
	if hashes == nil {
		hashes = review.ContentHashes(m.repo, files)
	}
	stale := staleKeys(m.files, files, m.hashes(), hashes)
	m.files = files
	if m.marks != nil {
		m.marks.hashes = hashes
	}
	prevKey := m.list.SelectedKey()
	m.list = m.list.SetListTitle(m.listTitle())
	var cmd tea.Cmd
	m.list, cmd = m.list.SetItemsSelectingStale(m.displayFiles(), prevKey, stale)
	return m, cmd
}

// hashes is the current content-hash map (empty when reviewing is off).
func (m Model) hashes() map[string]string {
	if m.marks == nil {
		return nil
	}
	return m.marks.hashes
}

// staleKeys is the set of preview cache keys invalidated by a file-set change:
// files whose record (status/stats) or content hash differs, plus files that
// disappeared, plus the synthetic "All changes" entry (keyed ""), whose
// preview aggregates everything.
func staleKeys(old, new []git.ChangedFile, oldHashes, newHashes map[string]string) map[string]bool {
	stale := map[string]bool{"": true}
	prev := make(map[string]git.ChangedFile, len(old))
	for _, f := range old {
		prev[f.Path] = f
	}
	seen := make(map[string]bool, len(new))
	for _, f := range new {
		seen[f.Path] = true
		if p, ok := prev[f.Path]; !ok || p != f || oldHashes[f.Path] != newHashes[f.Path] {
			stale[f.Path] = true
		}
	}
	for path := range prev {
		if !seen[path] {
			stale[path] = true
		}
	}
	return stale
}

func (m Model) View() tea.View { return m.list.TeaView() }

// Filtering and ShowingHelp expose the embedded list's modal state, so a parent
// embedding this model (e.g. the log drilldown) knows when not to intercept esc.
func (m Model) Filtering() bool   { return m.list.Filtering() }
func (m Model) ShowingHelp() bool { return m.list.ShowingHelp() }

// CancelPreview stops this model's in-flight diff stream — used by the lens
// wrapper before hiding it, so a backgrounded difftastic run doesn't linger.
func (m Model) CancelPreview() tea.Model {
	m.stream.Cancel()
	m.stream = nil
	return m
}

// previewCmd streams the selected files' diffs: the selected file, or every file
// for the synthetic "All changes" entry, diffed in parallel and emitted in order
// so the first paints as soon as it's ready. An empty changeset streams nothing
// (ApplyStream resolves the loading state and caches the empty result).
func (m Model) previewCmd(reqID int) tea.Cmd {
	sel, ok := m.list.Selected()
	if !ok {
		return nil
	}
	files := previewFiles(sel, m.list.VisibleItems())
	opts := m.previewOpts()
	root, engine := m.repo.Root(), m.engines.Engine()
	return func() tea.Msg {
		if len(files) == 0 {
			return tui.StreamReadyMsg{ReqID: reqID}
		}
		ch, cancel := tui.StreamFiles(engine, root, files, opts)
		return tui.StreamReadyMsg{ReqID: reqID, Ch: ch, Cancel: cancel}
	}
}

func (m Model) previewOpts() diff.DiffOpts {
	o := tui.PreviewDiffOpts(m.list.PreviewWidth(), m.display)
	o.Staged, o.Base, o.Target = m.staged, m.base, m.target
	return o
}

// previewFiles is the list of files a selection previews: just the selected file,
// or every changed file for the synthetic "All changes" entry (empty path).
func previewFiles(sel git.ChangedFile, visible []git.ChangedFile) []git.ChangedFile {
	if sel.Path != "" {
		return []git.ChangedFile{sel}
	}
	var files []git.ChangedFile
	for _, f := range visible {
		if f.Path != "" {
			files = append(files, f)
		}
	}
	return files
}
