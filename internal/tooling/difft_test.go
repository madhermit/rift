package tooling

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// TestFailureBackoff covers the negative cache (item 2d): a recorded failure
// suppresses re-attempts within the backoff window, an aged marker doesn't, and
// clearing removes it — so a blackholed network doesn't stall every invocation.
func TestFailureBackoff(t *testing.T) {
	dest := filepath.Join(t.TempDir(), "bin", "difft")

	if failedRecently(dest) {
		t.Fatal("no marker yet: should not be failed")
	}

	recordFailure(dest)
	if !failedRecently(dest) {
		t.Fatal("a fresh failure should be within the backoff window")
	}

	// Age the marker past the window: it should no longer suppress a retry.
	old := time.Now().Add(-2 * failureBackoff)
	if err := os.Chtimes(failureMarker(dest), old, old); err != nil {
		t.Fatal(err)
	}
	if failedRecently(dest) {
		t.Error("a marker older than the backoff window should not suppress retry")
	}

	recordFailure(dest)
	clearFailure(dest)
	if failedRecently(dest) {
		t.Error("clearFailure should remove the marker")
	}
}

// TestExtractDifft covers pulling the difft binary out of a release-shaped tarball
// (a nested path, plus a decoy entry that must be ignored).
func TestExtractDifft(t *testing.T) {
	want := []byte("#!/fake difft binary\n")
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	for _, e := range []struct {
		name string
		body []byte
	}{
		{"README.md", []byte("docs")},
		{"difft-x86_64/difft", want},
	} {
		if err := tw.WriteHeader(&tar.Header{Name: e.name, Typeflag: tar.TypeReg, Mode: 0o755, Size: int64(len(e.body))}); err != nil {
			t.Fatal(err)
		}
		if _, err := tw.Write(e.body); err != nil {
			t.Fatal(err)
		}
	}
	tw.Close()
	gz.Close()

	got, err := extractDifft(&buf)
	if err != nil {
		t.Fatalf("extractDifft: %v", err)
	}
	if !bytes.Equal(got, want) {
		t.Errorf("extracted %q, want %q", got, want)
	}
}
