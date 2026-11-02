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
// file and hunk marks, each file's current blob hash, and its hunks' content
// keys. All are mutated in place — a toggle on a store, a fresh hash map after
// an edit, a fresh key index when the scope's hunks land — so the row closure
// that captures the *reviewMarks always reflects the current state.
type reviewMarks struct {
	state  *review.Reviewed
	hunks  *review.Hunks
	hashes map[string]string
	keys   map[string][]string // file path → its hunks' content keys, in diff order
}

func (rm *reviewMarks) reviewed(path string) bool {
	return rm != nil && rm.state.IsReviewed(path, rm.hashes[path])
}

// partlyReviewed reports whether some of a file's hunks are marked. It doesn't
// need to exclude "all of them": completing a file's hunks marks the file too
// (see toggleHunkReviewed), and the row renderer checks that first.
func (rm *reviewMarks) partlyReviewed(path string) bool {
	if rm == nil {
		return false
	}
	n, _ := rm.hunks.Reviewed(path, rm.keys[path])
	return n > 0
}

// HunksLoadedMsg carries the scope's hunk index, read off the event loop. It
// names the scope it was read for so a load in flight when the scope changed
// (the `s` staged toggle rebuilds this model) is dropped rather than applied to
// the wrong side of the index. It is exported so a host can route it back here
// when another view is showing: losing it would strand the cursor, since the
// read is only ever started once.
type HunksLoadedMsg struct {
	staged bool
	base   string
	target string
	hunks  map[string][]diff.Hunk
	// keys are the hunks' content identities, hashed alongside the read rather
	// than in the renderer: the row for a file with hunk marks asks about every
	// one of them, every frame, and hashing hunk bodies there would put the diff
	// through sha256 on each repaint.
	keys map[string][]string
}

// hunkCursor is the diff's hunk-granular position: the scope's hunks by path,
// and the display line where each hunk of the *current preview* begins. The
// preview shows an engine's rendering, not the unified diff it came from, so
// positions have to be resolved against the finished content (see indexHunks) —
// which is also why they're a cache rather than something the render carries.
type hunkCursor struct {
	byPath  map[string][]diff.Hunk // scope hunks by path; nil until loaded
	loaded  bool
	loading bool       // a read is in flight; keeps a reset and a reselect from racing two
	spots   []hunkSpot // the current preview's hunks, in display order
	// at is the hunk the reader is on, numbered from 1 — 0 means none, which is
	// also the state before a preview resolves, so the zero value is correct and
	// a reset never has to remember a sentinel.
	at int
}

// hunkSpot is one hunk's place in the current preview: the display line it
// begins at, plus what addresses its reviewed mark. path is the file's real
// path, not the display path a rename's section is labelled with, because that
// is what the mark store is keyed by; key is empty where rift couldn't identify
// the hunk, which makes the spot navigable but not markable.
type hunkSpot struct {
	line int
	path string
	key  string
}

type Model struct {
	repo    *git.Repo
	engines tui.EngineToggle
	list    tui.SplitList[git.ChangedFile]

	staged      bool
	base        string
	target      string
	commitDiff  bool
	display     diff.Display
	stream      *tui.PreviewStream // active "all changes" stream; nil otherwise
	interrupted bool               // CancelPreview stopped the stream before it finished

	files          []git.ChangedFile // the full changed set; the list shows a (filtered) view of it
	paths          []string          // pathspec scope (from `-- path...`), re-applied on reload
	marks          *reviewMarks      // nil for a commit diff — historical files don't get reviewed
	unreviewedOnly bool
	watching       bool // show the live-watch indicator in the header context
	drilled        bool // embedded under a breadcrumb that already names the target
	hunk           hunkCursor
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
			hunks:  review.LoadHunks(repo),
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

	hints := [][2]string{{"/", "filter"}, {"⇥", "read"}, {"{/}", "hunk"}}
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
		hints = append(hints, [2]string{"o", "open"}, [2]string{"r/R", "review file/hunk"}, [2]string{"U", "hide reviewed"})
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
		HunkNav:     true,
		// Match on both sides of a rename, so a moved file is findable by its old
		// name too (OldPath is empty for everything else); identity stays the
		// new-side path, so a plain path (e.g. from nextUnreviewed) selects it.
		Match: func(f git.ChangedFile) string { return f.Path + " " + f.OldPath },
		Key:   func(f git.ChangedFile) string { return f.Path },
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
// A file part-way through a hunk-by-hunk pass keeps its status letter — there is
// still something there to read — but wears the reviewed color, so it reads at a
// glance as started rather than untouched.
func fileRow(marks *reviewMarks) func(git.ChangedFile, int, bool) string {
	return func(f git.ChangedFile, w int, selected bool) string {
		if f.Path == "" {
			return rowWithStat(tui.TextStyle(selected).Render("∗ All changes"), tui.DiffStat(f.Added, f.Deleted), w)
		}
		glyph := tui.StatusStyle(f.Status).Render(git.StatusChar(f.Status))
		switch {
		case marks.reviewed(f.Path):
			glyph = reviewedGlyph.Render("\u2713") // standard check (U+2713), font-independent
		case marks.partlyReviewed(f.Path):
			glyph = reviewedGlyph.Render(git.StatusChar(f.Status))
		}
		prefix := glyph + " " + tui.IconField(f.Path)
		return pathRow(prefix, selected, f.DisplayPath(), tui.DiffStat(f.Added, f.Deleted), w)
	}
}

// displayFiles is the list the SplitList shows. In the default view it's every
// changed file under a synthetic "All" entry. In the unreviewed-only view it's
// just the not-yet-reviewed files with no "All" row; toggleReviewed re-selects
// the next unreviewed file when one drops out.
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
	target := m.target
	if m.drilled {
		target = "" // the breadcrumb already names it
	}
	return tui.ContextLabel(target, m.engines.Name(), m.watching)
}

// Settings reports the engine and layout this view renders with.
func (m Model) Settings() tui.ViewSettings {
	return tui.ViewSettings{Engines: m.engines, Display: m.display}
}

// WithSettings sets the engine and layout on a view that hasn't rendered yet.
func (m Model) WithSettings(s tui.ViewSettings) Model {
	m.engines, m.display = s.Engines, s.Display
	m.list = m.list.SetContext(m.contextLabel())
	return m
}

// ApplySettings brings an already-rendered view in line with s, re-rendering
// only if that changes anything.
func (m Model) ApplySettings(s tui.ViewSettings) (tea.Model, tea.Cmd) {
	if m.Settings().Same(s) {
		return m, nil
	}
	return m.WithSettings(s).reloadFresh()
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
	m.drilled = true
	m.list = m.list.WithBreadcrumb(screen).SetContext(m.contextLabel())
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
		title += " · unreviewed only"
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
		load := m.loadHunksIfNeeded()
		if msg.CacheHit {
			// Cached content is final, so the new selection's hunks resolve now.
			m.indexHunks()
			return m.refreshHunkCursor(), load
		}
		m.hunk.spots, m.hunk.at = nil, 0 // the outgoing preview's positions no longer apply
		return m.refreshHunkCursor(), tea.Batch(load, m.previewCmd(msg.ReqID))
	case tui.StreamReadyMsg:
		var cmd tea.Cmd
		m.list, m.stream, cmd = tui.ApplyStream(m.list, m.stream, msg)
		if msg.Ch == nil {
			m.indexHunks() // header-only/empty preview: nothing more will arrive
		}
		return m.refreshHunkCursor(), cmd
	case tui.ChunkMsg:
		done := m.stream.Owns(msg) && !msg.More
		var cmd tea.Cmd
		m.list, m.stream, cmd = tui.AdvanceStream(m.list, m.stream, msg)
		if done {
			// The last chunk landed: the rendered rows are stable, so hunk
			// positions resolved against them stay valid until the next load.
			m.indexHunks()
		}
		return m.refreshHunkCursor(), cmd
	case HunksLoadedMsg:
		m.hunk.loading = false
		if msg.staged != m.staged || msg.base != m.base || msg.target != m.target {
			return m, nil // read for a scope this model has since left
		}
		m.hunk.byPath, m.hunk.loaded = msg.hunks, true
		if m.marks != nil {
			m.marks.keys = msg.keys
		}
		m.indexHunks()
		return m.refreshHunkCursor(), nil
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
		if m.list.Capturing() || m.list.ShowingHelp() {
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
		case "R":
			// The hunk cursor is a reading-mode position, so `R` acts where it is
			// visible: the list pane reviews files (`r`), the preview reviews hunks.
			if m.marks != nil && m.canNavigateHunks() {
				return m.toggleHunkReviewed()
			}
		case "U":
			if m.marks != nil {
				return m.toggleUnreviewedOnly()
			}
		case "}":
			if m.canNavigateHunks() {
				return m.navigateHunk(1)
			}
		case "{":
			if m.canNavigateHunks() {
				return m.navigateHunk(-1)
			}
		}
	}
	var cmd tea.Cmd
	m.list, cmd = m.list.Update(msg)
	// Any scroll — a key, a wheel, a jump — can carry the reader into another
	// hunk, so the cursor is re-synced here rather than only where it's stepped.
	return m.refreshHunkCursor(), cmd
}

// canNavigateHunks reports whether {/} should step hunks: only while reading
// (the preview is where they'd land) and only once its hunks are resolved.
// Otherwise the keys fall through to the list, which steps file sections —
// hunks when rift knows them, files when it doesn't.
func (m Model) canNavigateHunks() bool {
	return m.list.Reading() && len(m.hunk.spots) > 0
}

// navigateHunk steps the cursor to the next (dir > 0) or previous hunk and
// scrolls it to the top of the preview. The scroll is best-effort: at the end of
// a short diff the viewport clamps, so the last hunks share a screen. That's why
// the cursor is state rather than a function of the scroll position — several
// hunks can occupy the same position, and stepping has to keep moving anyway.
func (m Model) navigateHunk(dir int) (tea.Model, tea.Cmd) {
	next := m.hunk.at + dir
	if next < 1 || next > len(m.hunk.spots) {
		return m, nil // already at the first/last hunk
	}
	m.hunk.at = next
	m.list = m.list.ScrollPreviewTo(m.hunk.spots[next-1].line)
	return m.refreshHunkCursor(), nil
}

// syncHunkCursor re-derives the cursor from the scroll position after a free
// scroll — but only once the cursor's own hunk has left the screen. Near the end
// of a short diff several hunks are visible at once and the position alone can't
// say which one the reader means, so an explicit {/} step holds until it
// actually scrolls out of view.
func (m *Model) syncHunkCursor() {
	if len(m.hunk.spots) == 0 {
		m.hunk.at = 0
		return
	}
	y := m.list.PreviewOffset()
	if m.hunk.at > 0 && m.hunk.at <= len(m.hunk.spots) {
		if off := m.hunk.spots[m.hunk.at-1].line; off >= y && off < y+m.list.PreviewHeight() {
			return
		}
	}
	m.hunk.at = hunkAt(m.hunk.spots, y)
}

// refreshHunkCursor re-syncs the cursor to where the reader actually is and
// writes the "hunk N/M" readout into the preview legend, ticked when the hunk
// under the cursor is marked reviewed. The readout is empty — restoring the file
// position the legend otherwise shows — while a preview streams, when its
// content has no hunks, or above the first one.
func (m Model) refreshHunkCursor() Model {
	m.syncHunkCursor()
	m.list = m.list.SetPreviewStatus(m.hunkStatus())
	return m
}

// hunkStatus is the cursor's readout, ticked when the hunk under it is marked
// reviewed; empty when the cursor is on no hunk.
func (m Model) hunkStatus() string {
	if m.hunk.at < 1 || m.hunk.at > len(m.hunk.spots) {
		return ""
	}
	status := fmt.Sprintf("hunk %d/%d", m.hunk.at, len(m.hunk.spots))
	if m.hunkReviewed(m.hunk.spots[m.hunk.at-1]) {
		status += " ✓"
	}
	return status
}

// hunkReviewed reports whether a spot's hunk is marked. False whenever review
// state is off (a commit diff) or the hunk couldn't be identified.
func (m Model) hunkReviewed(s hunkSpot) bool {
	return m.marks != nil && m.marks.hunks.IsReviewed(s.path, s.key)
}

// hunkAt is the hunk the reader is inside — the last one beginning at or above
// the top row, numbered from 1 — or 0 when there are none, or the reader is
// above the first (a preview can open on a file header).
func hunkAt(spots []hunkSpot, y int) int {
	at := 0
	for i, s := range spots {
		if s.line > y {
			break
		}
		at = i + 1
	}
	return at
}

// loadHunksIfNeeded reads the scope's hunk index off the event loop, once per
// file set. Both triggers are needed and can coincide — a refresh resets the
// cursor, and the reselect it may cause asks again — so an in-flight read is
// tracked to keep them from spending two subprocesses on the same answer. It's
// an enhancement over the rendered preview — the diff shows either way — so a
// failure degrades to an unavailable cursor, never an error.
func (m *Model) loadHunksIfNeeded() tea.Cmd {
	if m.hunk.loaded || m.hunk.loading {
		return nil
	}
	m.hunk.loading = true
	root := m.repo.Root()
	opts := diff.DiffOpts{Staged: m.staged, Base: m.base, Target: m.target}
	// git diff omits untracked files, so they each need their own no-index diff.
	var untracked []string
	for _, f := range m.files {
		if f.Status == "Untracked" {
			untracked = append(untracked, f.Path)
		}
	}
	return func() tea.Msg {
		byPath := diff.ScopeHunks(root, opts)
		for _, path := range untracked {
			raw, err := diff.RawNewFileDiff(root, path)
			if err != nil {
				continue
			}
			var hunks []diff.Hunk
			for _, fd := range diff.ParseUnifiedDiff(raw) {
				hunks = append(hunks, fd.Hunks...)
			}
			if len(hunks) > 0 {
				byPath[path] = hunks // key by the path asked for, not the --no-index header
			}
		}
		keys := make(map[string][]string, len(byPath))
		for path, hunks := range byPath {
			ks := make([]string, len(hunks))
			for i, h := range hunks {
				ks[i] = review.HunkKey(h)
			}
			keys[path] = ks
		}
		return HunksLoadedMsg{staged: opts.Staged, base: opts.Base, target: opts.Target, hunks: byPath, keys: keys}
	}
}

// indexHunks resolves where each of the current preview's hunks begins in the
// rendered content, file by file — new-side line numbers restart per file, so
// each file's hunks are resolved within its own section. It runs only on
// finished content: rows in a half-streamed preview are still moving.
func (m *Model) indexHunks() {
	m.hunk.spots, m.hunk.at = nil, 0
	sel, ok := m.list.Selected()
	if !m.hunk.loaded || !ok {
		return
	}
	lines, sections := m.list.PreviewIndex()
	if len(lines) == 0 {
		return // nothing rendered yet; positions would be invented, not resolved
	}
	for _, f := range previewFiles(sel, m.list.VisibleItems()) {
		hunks := m.hunk.byPath[f.Path]
		if len(hunks) == 0 {
			continue
		}
		var keys []string
		if m.marks != nil {
			keys = m.marks.keys[f.Path] // absent for a commit diff: spots stay unmarkable
		}
		start, end := tui.SectionRange(sections, f.DisplayPath(), len(lines))
		for i, off := range tui.HunkOffsets(lines[start:end], hunks) {
			spot := hunkSpot{line: start + off, path: f.Path}
			if i < len(keys) {
				spot.key = keys[i]
			}
			m.hunk.spots = append(m.hunk.spots, spot)
		}
	}
}

// toggleReviewed flips the selected file's reviewed mark. Marking a file
// advances to the next unreviewed one (wrapping around), so a review pass is
// r-r-r rather than r-j-r-j; un-marking stays put — going back to a file to
// unmark it is deliberate. In the unreviewed-only view the file instead drops
// out of the list, which advances by itself.
func (m Model) toggleReviewed() (tea.Model, tea.Cmd) {
	sel, ok := m.list.Selected()
	if m.marks == nil || !ok || sel.Path == "" {
		return m, nil
	}
	if m.marks.state.Toggle(sel.Path, m.marks.hashes[sel.Path]) {
		return m.afterFileMarked(sel.Path)
	}
	// Un-marking: stay on the file the reviewer came back to. The unreviewed-only
	// view needs no rebuild — everything it shows is already unreviewed, so the
	// file is where it was.
	m.list = m.list.SetListTitle(m.listTitle())
	return m, nil
}

// afterFileMarked settles the list once path's file mark has gone on, whether
// that came from `r` or from `R` completing the file's last hunk: retitle, then
// move to the next file still needing eyes.
func (m Model) afterFileMarked(path string) (tea.Model, tea.Cmd) {
	m.list = m.list.SetListTitle(m.listTitle())
	next := nextUnreviewed(m.list.VisibleItems(), path, m.marks.reviewed)

	var cmd tea.Cmd
	if m.unreviewedOnly {
		// The marked file drops out of this view; keep the cursor on the next
		// unreviewed file after it (not the top — marking from the middle of the
		// list must not yank the reviewer back to row one). An empty next just
		// resets to the top, which is right once the view empties out.
		m.list, cmd = m.list.SetItemsSelecting(m.displayFiles(), next)
		return m.reselected(), cmd
	}
	if next == "" {
		m.list = m.list.SetFlash("all changes reviewed")
		return m, nil // the reader stays on this preview, so its cursor still holds
	}
	m.list, cmd = m.list.SelectKey(next)
	return m.reselected(), cmd
}

// reselected drops the hunk cursor after a reselect has swapped the preview out
// from under it. The incoming SelectionChangedMsg re-resolves it against the new
// content — but that is a message away, and a *cached* preview paints
// immediately, so without this the finished file's "hunk 2/2 ✓" would sit over
// the next file's diff for a frame.
func (m Model) reselected() Model {
	m.hunk.spots, m.hunk.at = nil, 0
	return m.refreshHunkCursor()
}

// toggleHunkReviewed flips the reviewed mark on the hunk under the cursor and
// keeps the file's own mark in step with it: marking a file's last open hunk
// marks the file, so a hunk-by-hunk pass ticks files off the list exactly as `r`
// does, and reopening any hunk reopens its file, so a ✓ never sits over code the
// reviewer just put back on the pile. Marking a file with `r` deliberately does
// not mark its hunks — reviewing a file whole is a claim about the file, not a
// claim about each hunk in it.
func (m Model) toggleHunkReviewed() (tea.Model, tea.Cmd) {
	if m.marks == nil {
		return m, nil
	}
	if m.hunk.at < 1 || m.hunk.at > len(m.hunk.spots) {
		// A preview opens on the file header, above the first hunk, so there is
		// nothing under the cursor yet. Start the pass instead of doing nothing:
		// land on the first hunk that still needs eyes.
		return m.advanceHunk()
	}
	spot := m.hunk.spots[m.hunk.at-1]
	if spot.key == "" {
		return m, nil // a hunk rift couldn't identify; navigable, but not markable
	}
	marked := m.marks.hunks.Toggle(spot.path, spot.key)
	hash := m.marks.hashes[spot.path]
	switch {
	case marked:
		if _, all := m.marks.hunks.Reviewed(spot.path, m.marks.keys[spot.path]); all {
			m.marks.state.Set(spot.path, hash, true)
		}
	case m.marks.reviewed(spot.path):
		m.marks.state.Set(spot.path, hash, false)
	}
	// A whole-file preview that just filled up settles like `r` does: on to the
	// next file needing eyes. In the "All changes" preview the reader isn't on any
	// one file, so the cursor stays and steps to the next open hunk — which walks
	// into the following file by itself. (afterFileMarked retitles for itself.)
	sel, ok := m.list.Selected()
	if marked && ok && sel.Path == spot.path && m.marks.reviewed(spot.path) {
		return m.afterFileMarked(spot.path)
	}
	m.list = m.list.SetListTitle(m.listTitle())
	if !marked {
		return m.refreshHunkCursor(), nil
	}
	return m.advanceHunk()
}

// advanceHunk steps the cursor to the next hunk in this preview that still needs
// eyes, so a pass reads R-R-R rather than R-}-R-}. It only looks forward:
// wrapping would march the reviewer back over hunks they deliberately left open.
func (m Model) advanceHunk() (tea.Model, tea.Cmd) {
	for i := m.hunk.at; i < len(m.hunk.spots); i++ { // at is 1-based, so this starts past it
		if s := m.hunk.spots[i]; !m.hunkReviewed(s) {
			m.hunk.at = i + 1
			m.list = m.list.ScrollPreviewTo(s.line)
			return m.refreshHunkCursor(), nil
		}
	}
	m.list = m.list.SetFlash("no unreviewed hunks below")
	return m.refreshHunkCursor(), nil
}

// nextUnreviewed returns the path of the first file after cur (wrapping) that
// isn't reviewed, skipping the synthetic All row; "" when every file is reviewed.
func nextUnreviewed(items []git.ChangedFile, cur string, reviewed func(string) bool) string {
	start := 0
	for i, f := range items {
		if f.Path == cur {
			start = i
			break
		}
	}
	for off := 1; off <= len(items); off++ {
		f := items[(start+off)%len(items)]
		if f.Path == "" || f.Path == cur {
			continue
		}
		if !reviewed(f.Path) {
			return f.Path
		}
	}
	return ""
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
	m.interrupted = false // this reload replaces whatever was cut short
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
	// The changed content re-cuts the hunks. Re-read them explicitly rather than
	// waiting for a selection change: an in-place refresh that keeps the reader
	// on an untouched file deliberately doesn't re-request its preview, and the
	// cursor must not go stale (or dead) just because nothing was re-selected.
	m.hunk = hunkCursor{}
	if m.marks != nil {
		m.marks.keys = nil // re-cut hunks mean re-hashed keys; the old ones name nothing
	}
	prevKey := m.list.SelectedKey()
	m.list = m.list.SetListTitle(m.listTitle())
	var cmd tea.Cmd
	m.list, cmd = m.list.SetItemsSelectingStale(m.displayFiles(), prevKey, stale)
	return m, tea.Batch(cmd, m.loadHunksIfNeeded())
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

// Capturing, ShowingHelp, and InMode expose the embedded list's modal state to
// a parent hosting this model (the log drilldown, the files↔tests lens): when
// not to run its own key handlers, and when not to take esc for itself.
func (m Model) Capturing() bool   { return m.list.Capturing() }
func (m Model) InMode() bool      { return m.list.InMode() }
func (m Model) ShowingHelp() bool { return m.list.ShowingHelp() }

// CancelPreview stops this model's in-flight diff stream — used by the lens
// wrapper before hiding it, so a backgrounded difftastic run doesn't linger.
func (m Model) CancelPreview() tea.Model {
	m.interrupted = m.stream != nil
	m.stream.Cancel()
	m.stream = nil
	return m
}

// ResumePreview reloads a preview that being hidden cut short — a load that never
// answered, or a stream stopped part-way. A finished one is left alone, so the
// reader comes back to the place they left.
func (m Model) ResumePreview() (tea.Model, tea.Cmd) {
	if !m.interrupted && !m.list.Loading() {
		return m, nil
	}
	m.interrupted = false
	var cmd tea.Cmd
	m.list, cmd = m.list.Reload()
	return m, cmd
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
