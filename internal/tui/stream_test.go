package tui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
)

func TestWaitChunk(t *testing.T) {
	ch := make(chan string, 1)
	ch <- "hello"
	if msg := WaitChunk(ch)().(ChunkMsg); !msg.More || msg.Content != "hello" || msg.Ch != ch {
		t.Errorf("got %+v", msg)
	}
	close(ch)
	if msg := WaitChunk(ch)().(ChunkMsg); msg.More {
		t.Error("a closed channel should yield More=false")
	}
}

func TestPreviewStream(t *testing.T) {
	ch := make(chan string, 1)
	cancelled := false
	s, cmd := NewPreviewStream(7, ch, func() { cancelled = true })
	if cmd == nil {
		t.Fatal("NewPreviewStream should return a wait command")
	}

	// Ownership is by channel identity; a nil stream owns nothing.
	if !s.Owns(ChunkMsg{Ch: ch}) {
		t.Error("should own a chunk from its own channel")
	}
	if s.Owns(ChunkMsg{Ch: make(chan string)}) {
		t.Error("should not own a chunk from a foreign channel")
	}
	if (*PreviewStream)(nil).Owns(ChunkMsg{Ch: ch}) {
		t.Error("a nil stream owns nothing")
	}

	// A content chunk yields an Append PreviewMsg carrying the stream's reqID.
	out, next, done := s.take(ChunkMsg{Content: "x", More: true, Ch: ch})
	if done || next == nil || !out.Append || out.ReqID != 7 || out.Content != "x" {
		t.Errorf("content chunk: out=%+v done=%v next==nil:%v", out, done, next == nil)
	}
	// Exhaustion reports done with no further message.
	if _, next, done := s.take(ChunkMsg{More: false, Ch: ch}); !done || next != nil {
		t.Errorf("More=false should report done with no command (done=%v next==nil:%v)", done, next == nil)
	}

	// Cancel runs the cancel func; it's safe on a nil stream.
	s.Cancel()
	if !cancelled {
		t.Error("Cancel should invoke the cancel func")
	}
	(*PreviewStream)(nil).Cancel() // must not panic
}

// TestAdvanceStream covers the shared chunk handler: a foreign chunk is not
// handled; a content chunk appends; exhaustion cancels, caches, and clears.
func TestAdvanceStream(t *testing.T) {
	m := NewSplitList(testCfg(), []string{"alpha"})
	m, _ = m.Update(tea.WindowSizeMsg{Width: 56, Height: 24})
	ch := make(chan string, 1)
	cancelled := false
	s, _ := NewPreviewStream(m.reqID, ch, func() { cancelled = true })

	// Foreign chunk: ignored, stream and command unchanged.
	if _, st, cmd := AdvanceStream(m, s, ChunkMsg{Ch: make(chan string)}); st != s || cmd != nil {
		t.Error("a foreign chunk should be ignored (stream kept, nil command)")
	}
	// Content chunk: appended, stream kept.
	var cmd tea.Cmd
	m, s, cmd = AdvanceStream(m, s, ChunkMsg{Content: "C1\n", More: true, Ch: ch})
	if s == nil || cmd == nil {
		t.Fatal("content chunk should keep the stream and re-arm the wait")
	}
	if !strings.Contains(plainView(m), "C1") {
		t.Error("content chunk should be appended to the preview")
	}
	// Exhaustion: stream cleared and cancelled.
	m, s, _ = AdvanceStream(m, s, ChunkMsg{More: false, Ch: ch})
	if s != nil || !cancelled {
		t.Errorf("exhaustion should clear (nil) and cancel the stream (s==nil:%v cancelled:%v)", s == nil, cancelled)
	}
}

// TestCacheCurrentPreview verifies streamed (Append) chunks are not cached
// individually, but CacheCurrentPreview memoizes the accumulated result so a
// revisit is an instant cache hit rather than a reload.
func TestCacheCurrentPreview(t *testing.T) {
	m := NewSplitList(testCfg(), []string{"alpha", "beta"})
	m, _ = m.Update(tea.WindowSizeMsg{Width: 56, Height: 24})

	m, _ = m.Update(PreviewMsg{Content: "A1\n", ReqID: m.reqID, Append: true})
	m, _ = m.Update(PreviewMsg{Content: "A2\n", ReqID: m.reqID, Append: true})
	m = m.CacheCurrentPreview()

	// Move to beta, then back to alpha: alpha is a cache hit (no spinner).
	m, _ = m.Update(tea.KeyPressMsg{Code: 'j', Text: "j"})
	m, _ = m.Update(PreviewMsg{Content: "BETA", ReqID: m.reqID})
	m, _ = m.Update(tea.KeyPressMsg{Code: 'k', Text: "k"})
	if m.loading {
		t.Error("returning to alpha should hit the stream cache, not reload")
	}
	if v := plainView(m); !strings.Contains(v, "A1") || !strings.Contains(v, "A2") {
		t.Error("the cached streamed preview should be restored in full")
	}
}
