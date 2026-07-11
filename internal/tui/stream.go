package tui

import (
	"context"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/madhermit/rift/internal/diff"
	"github.com/madhermit/rift/internal/git"
)

// ChunkMsg carries one item's rendered preview from a progressive stream. More
// is false when the stream is exhausted. Ch identifies the originating stream so
// a chunk from a superseded stream — e.g. a previously-selected commit whose
// per-model request counter happens to collide — is dropped rather than applied
// to the wrong preview.
type ChunkMsg struct {
	Content string
	More    bool
	Ch      <-chan string
}

// WaitChunk blocks for the next value on ch and wraps it as a ChunkMsg.
func WaitChunk(ch <-chan string) tea.Cmd {
	return func() tea.Msg {
		content, ok := <-ch
		return ChunkMsg{Content: content, More: ok, Ch: ch}
	}
}

// PreviewDiffOpts builds the rendering options common to every preview: color
// per NO_COLOR, the pane width, and the layout mode. Callers layer Base/Target
// (and, for the working tree, Staged) on top.
func PreviewDiffOpts(width int, display diff.Display) diff.DiffOpts {
	return diff.DiffOpts{Color: ColorEnabled(), Width: width, Display: display}
}

// StreamFiles diffs files concurrently and returns a channel yielding each
// file's rendered diff in order (for a progressive preview), plus a cancel that
// stops the work — killing the difftastic subprocesses of a stream the user
// navigated away from. Shared by the diff, log, and stash previews. Each file's
// OldPath (when renamed) rides into the engine via the per-file opts.
func StreamFiles(engine diff.Engine, root string, files []git.ChangedFile, opts diff.DiffOpts) (<-chan string, func()) {
	ctx, cancel := context.WithCancel(context.Background())
	ch := diff.ParallelStream(len(files), func(i int) string {
		o := opts
		o.OldPath = files[i].OldPath
		return renderFileDiff(ctx, engine, root, files[i], o)
	})
	return ch, cancel
}

// renderFileDiff diffs one file, prefixing a section banner. The banner is a
// hidden marker — the preview strips it from the body (the diff's own header is
// the visible boundary) and pins its label in the legend as you scroll into the
// file (a rename reads "old → new"); see VimNav.SetContent. A file whose diff
// fails renders a visible marker rather than vanishing silently; a cancelled
// diff (navigated away) renders nothing, since it's discarded.
func renderFileDiff(ctx context.Context, engine diff.Engine, root string, f git.ChangedFile, opts diff.DiffOpts) string {
	content, err := engine.Diff(ctx, root, f.Path, opts)
	label := f.DisplayPath()
	switch {
	case ctx.Err() != nil:
		return ""
	case err != nil:
		return diffErrorMarker(label, opts, err)
	case content == "":
		return ""
	default:
		return SectionBanner(label, opts.Width) + "\n" + content + "\n"
	}
}

func diffErrorMarker(label string, opts diff.DiffOpts, err error) string {
	line := strings.SplitN(err.Error(), "\n", 2)[0]
	return SectionBanner(label, opts.Width) + "\n  ⚠ diff unavailable: " + line + "\n"
}

// PreviewStream tracks an in-flight progressive preview feeding a SplitList: a
// channel of ordered, already-rendered chunks (from StreamFiles) appended to the
// preview as they arrive. The first chunk appends onto the pane that
// requestPreview cleared (and scrolled to top), so chunks never reset the scroll
// position; the accumulated result is cached on completion via
// SplitList.CacheCurrentPreview. Cancel stops the underlying work.
type PreviewStream struct {
	reqID  int
	ch     <-chan string
	cancel func()
}

// NewPreviewStream tracks ch as the preview stream for reqID and returns the
// command that waits for its first chunk. cancel (may be nil) stops the work
// producing ch when the stream is superseded or finished.
func NewPreviewStream(reqID int, ch <-chan string, cancel func()) (*PreviewStream, tea.Cmd) {
	return &PreviewStream{reqID: reqID, ch: ch, cancel: cancel}, WaitChunk(ch)
}

// Cancel stops the stream's underlying work. Safe on a nil stream or nil cancel.
func (s *PreviewStream) Cancel() {
	if s != nil && s.cancel != nil {
		s.cancel()
	}
}

// Owns reports whether msg belongs to this stream (vs a superseded one). Safe on
// a nil stream.
func (s *PreviewStream) Owns(msg ChunkMsg) bool {
	return s != nil && msg.Ch == s.ch
}

// take consumes a chunk known to belong to this stream. done is true when the
// stream is exhausted; otherwise out is the message to feed the list and next
// re-arms the wait for the following chunk.
func (s *PreviewStream) take(msg ChunkMsg) (out PreviewMsg, next tea.Cmd, done bool) {
	if !msg.More {
		return PreviewMsg{}, nil, true
	}
	return PreviewMsg{Content: msg.Content, ReqID: s.reqID, Append: true}, WaitChunk(s.ch), false
}

// StreamReadyMsg is produced off the UI thread once a screen has computed what to
// stream for a selection: an optional Header rendered first, the channel of
// ordered chunks (nil for a header-only/empty preview), and a Cancel for that
// work. ApplyStream consumes it.
type StreamReadyMsg struct {
	ReqID  int
	Header string
	Ch     <-chan string
	Cancel func()
}

// ApplyStream installs a freshly-loaded streamed preview. If ReqID is stale — the
// user navigated on while the file list was computed off-thread — it cancels the
// load and leaves the current stream untouched, so a late load can't clobber the
// live stream or cache the wrong preview. Otherwise it appends the header (if
// any), then either caches a header-only/empty result (Ch == nil, also emitting an
// empty message so loading clears) or installs Ch as the new stream.
func ApplyStream[T any](list SplitList[T], stream *PreviewStream, msg StreamReadyMsg) (SplitList[T], *PreviewStream, tea.Cmd) {
	if msg.ReqID != list.reqID {
		if msg.Cancel != nil {
			msg.Cancel()
		}
		return list, stream, nil
	}
	var lc tea.Cmd
	if msg.Header != "" || msg.Ch == nil {
		list, lc = list.Update(PreviewMsg{Content: msg.Header, ReqID: msg.ReqID, Append: true})
	}
	if msg.Ch == nil {
		return list.CacheCurrentPreview(), nil, lc
	}
	ns, sc := NewPreviewStream(msg.ReqID, msg.Ch, msg.Cancel)
	return list, ns, tea.Batch(lc, sc)
}

// AdvanceStream applies a streamed chunk to a list. A chunk that doesn't belong
// to stream leaves everything unchanged (and returns a nil command). Otherwise it
// is appended; when the stream is exhausted it is cancelled, the accumulated
// preview is cached, and the returned stream is nil. This is the single home for
// the models' otherwise-identical ChunkMsg handling.
func AdvanceStream[T any](list SplitList[T], stream *PreviewStream, msg ChunkMsg) (SplitList[T], *PreviewStream, tea.Cmd) {
	if !stream.Owns(msg) {
		return list, stream, nil
	}
	out, next, done := stream.take(msg)
	if done {
		stream.Cancel()
		return list.CacheCurrentPreview(), nil, nil
	}
	list, cmd := list.Update(out)
	return list, stream, tea.Batch(cmd, next)
}
