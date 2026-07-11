package tooling

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"time"

	"golang.org/x/term"
)

// difftVersion pins the difftastic release rift downloads. Pinning (rather than
// releases/latest) keeps every diff on a known-good binary — an upstream flag or
// output change can't silently break rendering. The per-asset sha256 sums below
// are for this exact tag; bump both together when upgrading.
const difftVersion = "0.67.0"

// difftAsset is a release archive name plus the sha256 of the downloaded
// tarball. An empty sum means "unverified" (checksums couldn't be embedded); the
// download still proceeds and is validated by running the binary.
type difftAsset struct {
	name   string
	sha256 string
}

var difftAssets = map[string]difftAsset{
	"linux/amd64":  {"difft-x86_64-unknown-linux-gnu.tar.gz", "61c17cfc6525236529f075821073cc0d8a65ae5abe1b81e9b24d9f36797b6b59"},
	"linux/arm64":  {"difft-aarch64-unknown-linux-gnu.tar.gz", "e36f746fbf5d37c68c1d760878fdb77778066c6a646253bb8d9ae7d3fe84c81b"},
	"darwin/amd64": {"difft-x86_64-apple-darwin.tar.gz", "83cc28d781f11b7fc2723b73ded623cf97a70a0849dff1efa39d1864adfdac54"},
	"darwin/arm64": {"difft-aarch64-apple-darwin.tar.gz", "cdb1d74f2b0f37df3bce7a9ac09d8ecd0d2d94530627516ff9b12f25d187a555"},
}

// downloadTimeout bounds a single install attempt end to end.
const downloadTimeout = 30 * time.Second

// failureBackoff is how long a failed install is remembered so a blackholed
// network doesn't stall every rift invocation retrying synchronously.
const failureBackoff = time.Hour

// DataDir is rift's per-user data directory (managed binaries, the agent
// skill). Kept in one place so a future XDG_DATA_HOME override lands
// everywhere at once.
func DataDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".local", "share", "rift"), nil
}

func managedPath() (string, error) {
	dir, err := DataDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "bin", "difft"), nil
}

func FindOrInstallDifft() (string, error) {
	if path, err := exec.LookPath("difft"); err == nil {
		return path, nil
	}

	managed, err := managedPath()
	if err != nil {
		return "", err
	}

	// Trust a pre-existing managed binary only if it actually runs — a truncated
	// or half-written file from an interrupted or racing install would otherwise
	// be trusted forever, keeping the line-diff fallback from ever engaging.
	if _, statErr := os.Stat(managed); statErr == nil && difftRuns(managed) {
		return managed, nil
	}

	// No usable binary. Respect the negative-cache window before re-attempting.
	if failedRecently(managed) {
		return "", errors.New("difftastic install skipped: a recent attempt failed")
	}

	if err := installDifft(managed); err != nil {
		recordFailure(managed)
		return "", err
	}
	clearFailure(managed)
	return managed, nil
}

func installDifft(dest string) error {
	key := runtime.GOOS + "/" + runtime.GOARCH
	asset, ok := difftAssets[key]
	if !ok {
		return fmt.Errorf("unsupported platform: %s", key)
	}
	url := fmt.Sprintf("https://github.com/Wilfred/difftastic/releases/download/%s/%s", difftVersion, asset.name)

	stop := spinner(os.Stderr, "Installing difftastic")
	defer stop()

	data, err := download(url)
	if err != nil {
		return fmt.Errorf("download difftastic: %w", err)
	}
	if asset.sha256 != "" {
		sum := sha256.Sum256(data)
		if got := hex.EncodeToString(sum[:]); got != asset.sha256 {
			return fmt.Errorf("difftastic checksum mismatch for %s: got %s", asset.name, got)
		}
	}

	bin, err := extractDifft(bytes.NewReader(data))
	if err != nil {
		return fmt.Errorf("extract difftastic: %w", err)
	}

	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		return fmt.Errorf("create dir: %w", err)
	}

	// Write to a temp file in the same dir, validate it, then atomically rename
	// into place — an interrupted write or two racing rift processes never leave a
	// truncated binary at dest.
	tmp, err := os.CreateTemp(filepath.Dir(dest), ".difft-*")
	if err != nil {
		return fmt.Errorf("create temp: %w", err)
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName) // no-op once renamed away
	if _, err := tmp.Write(bin); err != nil {
		tmp.Close()
		return fmt.Errorf("write difft: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("write difft: %w", err)
	}
	if err := os.Chmod(tmpName, 0o755); err != nil {
		return fmt.Errorf("chmod difft: %w", err)
	}
	if !difftRuns(tmpName) {
		return errors.New("installed difftastic failed to run")
	}
	if err := os.Rename(tmpName, dest); err != nil {
		return fmt.Errorf("install difft: %w", err)
	}
	return nil
}

// download fetches url fully into memory (difft tarballs are a few MB) so the
// bytes can be checksummed before extraction. The client timeout bounds the whole
// attempt.
func download(url string) ([]byte, error) {
	client := &http.Client{Timeout: downloadTimeout}
	resp, err := client.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	return io.ReadAll(resp.Body)
}

// difftRuns reports whether the binary at path executes — the trust check for a
// managed install and the post-download validation.
func difftRuns(path string) bool {
	return exec.Command(path, "--version").Run() == nil
}

func failureMarker(dest string) string { return dest + ".failed" }

// failedRecently reports whether an install failed within the backoff window.
func failedRecently(dest string) bool {
	fi, err := os.Stat(failureMarker(dest))
	if err != nil {
		return false
	}
	return time.Since(fi.ModTime()) < failureBackoff
}

func recordFailure(dest string) {
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		return
	}
	if f, err := os.Create(failureMarker(dest)); err == nil {
		f.Close()
	}
}

func clearFailure(dest string) { _ = os.Remove(failureMarker(dest)) }

// spinner animates a progress line on w while an install runs. It no-ops when w
// isn't a terminal (so nothing bleeds into piped/redirected stderr), and stop()
// joins the goroutine so its final erase can't race later output.
func spinner(w *os.File, msg string) func() {
	if !term.IsTerminal(int(w.Fd())) {
		return func() {}
	}
	frames := []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}
	done := make(chan struct{})
	finished := make(chan struct{})
	go func() {
		defer close(finished)
		ticker := time.NewTicker(80 * time.Millisecond)
		defer ticker.Stop()
		i := 0
		for {
			select {
			case <-done:
				fmt.Fprintf(w, "\r\033[K")
				return
			case <-ticker.C:
				fmt.Fprintf(w, "\r%s %s", frames[i%len(frames)], msg)
				i++
			}
		}
	}()
	return func() {
		close(done)
		<-finished
	}
}

func extractDifft(r io.Reader) ([]byte, error) {
	gz, err := gzip.NewReader(r)
	if err != nil {
		return nil, err
	}
	defer gz.Close()

	tr := tar.NewReader(gz)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}
		if filepath.Base(hdr.Name) == "difft" && hdr.Typeflag == tar.TypeReg {
			return io.ReadAll(tr)
		}
	}
	return nil, fmt.Errorf("difft binary not found in archive")
}
