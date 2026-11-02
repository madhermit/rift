package logui

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/madhermit/rift/internal/diff"
	"github.com/madhermit/rift/internal/git"
	"github.com/madhermit/rift/internal/review"
	"github.com/madhermit/rift/internal/tui"
	lensui "github.com/madhermit/rift/internal/tui/lens"
)

// Action is a git operation the user chose to run on the selected commit; the
// program quits with it and the command layer performs it.
type Action int

const (
	NoAction Action = iota
	CherryPick
	Revert
)

type Model struct {
	repo     *git.Repo
	settings tui.ViewSettings
	list     tui.SplitList[git.CommitInfo]
	action   Action
	confirm  tui.Confirm        // pending y/n gate for cherry-pick / revert
	stream   *tui.PreviewStream // active commit-diff stream; nil otherwise
	drill    lensui.Drill       // the selected commit's files↔tests view, once opened
	tests    bool               // `⏎` opens a commit on its tests (--tests); `t` the other side
	width    int
	height   int
}

// Action reports the git operation chosen for the selected commit (NoAction if
// the user just quit).
func (m Model) Action() Action { return m.action }

func (a Action) verb() string {
	switch a {
	case CherryPick:
		return "cherry-pick"
	case Revert:
		return "revert"
	}
	return ""
}

// startAction arms cherry-pick / revert on the selected commit behind an inline
// y/n confirmation (both rewrite history / the working tree). resolveConfirm quits
// with the action on "y".
func (m Model) startAction(action Action) (tea.Model, tea.Cmd) {
	commit, ok := m.list.Selected()
	// Cherry-pick / revert fire only from the list pane, not while reading the
	// preview — matching stash's action gate.
	if !ok || m.list.Reading() {
		return m, nil
	}
	m.action = action
	m.confirm = tui.Ask(fmt.Sprintf("%s %s", action.verb(), commit.Hash))
	m.list = m.list.SetPrompt(m.confirm.Prompt())
	return m, nil
}

// resolveConfirm answers a pending y/n confirmation: "y" runs the armed action
// (quit), anything else cancels it.
func (m Model) resolveConfirm(key string) (tea.Model, tea.Cmd) {
	confirmed, cleared := m.confirm.Answer(key)
	m.confirm = cleared
	m.list = m.list.SetPrompt("")
	if confirmed {
		return m, tea.Quit
	}
	m.action = NoAction
	return m, nil
}

// SelectedHash is the hash of the commit the user acted on, or "" if none.
func (m Model) SelectedHash() string {
	if c, ok := m.list.Selected(); ok {
		return c.Hash
	}
	return ""
}

func New(repo *git.Repo, engine diff.Engine, commits []git.CommitInfo, tests bool) Model {
	settings := tui.ViewSettings{Engines: tui.NewEngineToggle(engine)}
	branch := repo.CurrentBranch()

	enterLabel, otherLabel := "files", "tests"
	if tests {
		enterLabel, otherLabel = otherLabel, enterLabel
	}
	hints := [][2]string{
		{"⏎", enterLabel}, {"t", otherLabel}, {"/", "filter"}, {"⇥", "read"}, {"\\", "layout"},
	}
	if settings.Engines.CanToggle() {
		hints = append(hints, [2]string{"e", "engine"})
	}
	hints = append(hints,
		[2]string{"c", "cherry-pick"}, [2]string{"r", "revert"},
		[2]string{"y", "yank"}, [2]string{"?", "help"}, [2]string{"q", "quit"})

	cfg := tui.SplitConfig[git.CommitInfo]{
		Screen:       "log",
		ListTitle:    "commits",
		Branch:       branch,
		Context:      engine.Name(),
		NavFraction:  30,
		EmptyStatus:  "No commits found",
		Hints:        hints,
		Match:        func(c git.CommitInfo) string { return c.Hash + " " + c.Message },
		PreviewTitle: func(c git.CommitInfo) string { return c.Hash },
		CacheKey:     func(c git.CommitInfo) string { return c.Hash },
		Yank:         func(c git.CommitInfo) string { return c.Hash },
		Row: func(c git.CommitInfo, w int, selected bool) string {
			return tui.TextStyle(selected).Render(tui.Truncate(c.Hash+"  "+c.Message, w))
		},
	}
	return Model{repo: repo, settings: settings, tests: tests, list: tui.NewSplitList(cfg, commits)}
}

func (m Model) Init() tea.Cmd { return tui.ThemeInit() }

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	// Everything but the commit list's own traffic belongs to an open drilldown —
	// input especially: a mouse event reaching the hidden list would silently move
	// its selection, and a later cherry-pick/revert would act on the drifted commit.
	if m.drill.Owns(msg) {
		next, cmd, closed := m.drill.Update(msg)
		m.drill = next
		if closed {
			m.list, m.settings, cmd = lensui.BackToList(m.drill, m.list, m.settings)
		}
		return m, cmd
	}
	if lensui.Discard(msg) {
		return m, nil // a late message from a closed drilldown
	}

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		// Goes to both, so the commit list is current when we return.
		m.width, m.height = msg.Width, msg.Height
		var lc, dc tea.Cmd
		m.list, lc = m.list.Update(msg)
		m.drill, dc = m.drill.Resize(msg)
		return m, tea.Batch(lc, dc)
	case tui.SelectionChangedMsg:
		m.stream.Cancel() // stop the previous commit's streaming work
		m.stream = nil
		if m.drill.Open() || msg.CacheHit {
			return m, nil // hidden, or served from cache; don't stream
		}
		return m, m.previewCmd(msg.ReqID)
	case tui.StreamReadyMsg:
		if m.drill.Open() {
			if msg.Cancel != nil {
				msg.Cancel() // a load that landed after we left the list; drop it
			}
			return m, nil
		}
		var cmd tea.Cmd
		m.list, m.stream, cmd = tui.ApplyStream(m.list, m.stream, msg)
		return m, cmd
	case tui.ChunkMsg:
		var cmd tea.Cmd
		m.list, m.stream, cmd = tui.AdvanceStream(m.list, m.stream, msg)
		return m, cmd
	case tea.KeyPressMsg:
		if m.list.Capturing() || m.list.ShowingHelp() {
			break
		}
		if m.confirm.Active() {
			return m.resolveConfirm(msg.String())
		}
		switch msg.String() {
		case "enter":
			return m.openDrill(m.tests)
		case "t":
			return m.openDrill(!m.tests)
		case "\\":
			m.settings.Display = m.settings.Display.Next()
			return m.reloadFresh()
		case "e":
			if !m.settings.Engines.CanToggle() {
				break // only one engine available; nothing to toggle
			}
			m.settings.Engines = m.settings.Engines.Toggle()
			m.list = m.list.SetContext(m.settings.Engines.Name())
			return m.reloadFresh()
		case "c":
			return m.startAction(CherryPick)
		case "r":
			return m.startAction(Revert)
		}
	}
	var cmd tea.Cmd
	m.list, cmd = m.list.Update(msg)
	return m, cmd
}

// openDrill opens the selected commit's files↔tests view, on its tests side when
// tests is set. The list's own preview stream is cancelled: it's hidden now, and
// shouldn't keep difftastic busy.
func (m Model) openDrill(tests bool) (tea.Model, tea.Cmd) {
	commit, ok := m.list.Selected()
	if !ok {
		return m, nil
	}
	m.stream.Cancel()
	m.stream = nil
	base, files, err := m.repo.CommitFiles(commit.Hash)
	scope := review.DiffScope{Base: base, Target: commit.Hash}
	lens := lensui.NewDrill(m.repo, m.settings, files, scope, tui.Breadcrumb("log", commit.Hash))
	if err != nil {
		// Surface the load failure instead of an indistinguishable "No changes found".
		lens = lens.SetEmptyStatus(fmt.Sprintf("could not load commit diff: %v", err))
	}
	var cmd tea.Cmd
	m.drill, cmd = lensui.OpenDrill(lens, m.width, m.height, tests)
	return m, cmd
}

// reloadFresh drops the preview cache and reloads the current selection, after a
// setting (layout/engine) that affects rendering has changed.
func (m Model) reloadFresh() (tea.Model, tea.Cmd) {
	var cmd tea.Cmd
	m.list, cmd = m.list.ClearCacheAndReload()
	return m, cmd
}

func (m Model) View() tea.View {
	if m.drill.Open() {
		return m.drill.View()
	}
	return m.list.TeaView()
}

// previewCmd builds the selected commit's preview. The commit header (metadata +
// file list) is shown as soon as the file list is computed; the per-file diffs
// then stream in beneath it, in order, diffed in parallel — so a large commit
// paints its header and first file immediately instead of blocking on the whole
// diff. The accumulated result is cached (per commit) when the stream completes.
func (m Model) previewCmd(reqID int) tea.Cmd {
	commit, ok := m.list.Selected()
	if !ok {
		return nil
	}
	repo, engine := m.repo, m.settings.Engines.Engine()
	opts := tui.PreviewDiffOpts(m.list.PreviewWidth(), m.settings.Display)
	return func() tea.Msg {
		base, files, err := repo.CommitFiles(commit.Hash)
		if err != nil {
			return tui.PreviewMsg{Err: err, ReqID: reqID}
		}
		header := commitHeader(commit, files, opts.Color, opts.Width)
		if len(files) == 0 {
			return tui.StreamReadyMsg{ReqID: reqID, Header: header}
		}
		opts.Base, opts.Target = base, commit.Hash
		ch, cancel := tui.StreamFiles(engine, repo.Root(), files, opts)
		return tui.StreamReadyMsg{ReqID: reqID, Header: header, Ch: ch, Cancel: cancel}
	}
}

func commitHeader(commit git.CommitInfo, files []git.ChangedFile, color bool, width int) string {
	hash := commit.Hash
	authorLabel := "Author:"
	dateLabel := "Date:"

	const indent = "    "
	wrapWidth := width - len(indent)

	subject := commit.Message
	if wrapWidth > 0 {
		subject = ansi.Wordwrap(subject, wrapWidth, "")
	}
	subject = indent + strings.ReplaceAll(subject, "\n", "\n"+indent)

	body := ""
	if commit.Body != "" {
		body = "\n" + tui.Markdown(commit.Body, width, color)
	}
	sep := "─────────────────────"

	if color {
		hash = hashStyle.Render(hash)
		authorLabel = headerLabelStyle.Render(authorLabel)
		dateLabel = headerLabelStyle.Render(dateLabel)
		subject = "\x1b[1m" + subject + "\x1b[22m"
		sep = headerLabelStyle.Render(sep)
	}

	var b strings.Builder
	fmt.Fprintf(&b, "commit %s\n%s %s\n%s   %s\n\n%s%s\n", hash, authorLabel, commit.Author, dateLabel, commit.Date, subject, body)

	if len(files) > 0 {
		b.WriteString("\n")
		for _, f := range files {
			status := git.StatusChar(f.Status)
			// IconField carries its own trailing space (empty when icons are off), so
			// the column drops out cleanly under RIFT_ICONS=ascii|none.
			iconField := tui.IconField(f.Path)
			// Reserve room for the "  <status> <icon> " prefix; fall back to the
			// full path when the width isn't known yet (unsized preview).
			pathWidth := width - 6
			if pathWidth < 1 {
				pathWidth = len(f.Path)
			}
			path := tui.RenderPath(f.Path, pathWidth, false)
			if color {
				status = tui.StatusStyle(f.Status).Render(status)
			} else {
				iconField = ansi.Strip(iconField)
				path = ansi.Strip(path)
			}
			fmt.Fprintf(&b, "  %s %s%s\n", status, iconField, path)
		}
	}

	fmt.Fprintf(&b, "\n%s\n\n", sep)
	return b.String()
}
