package tooling

import (
	"archive/tar"
	"compress/gzip"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
)

var difftAssets = map[string]string{
	"linux/amd64":  "difft-x86_64-unknown-linux-gnu.tar.gz",
	"linux/arm64":  "difft-aarch64-unknown-linux-gnu.tar.gz",
	"darwin/amd64": "difft-x86_64-apple-darwin.tar.gz",
	"darwin/arm64": "difft-aarch64-apple-darwin.tar.gz",
}

func managedPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".local", "share", "rift", "bin", "difft"), nil
}

func FindOrInstallDifft() (string, error) {
	if path, err := exec.LookPath("difft"); err == nil {
		return path, nil
	}

	managed, err := managedPath()
	if err != nil {
		return "", err
	}

	if _, err := os.Stat(managed); err == nil {
		return managed, nil
	}

	return downloadDifft(managed)
}

func downloadDifft(dest string) (string, error) {
	key := runtime.GOOS + "/" + runtime.GOARCH
	asset, ok := difftAssets[key]
	if !ok {
		return "", fmt.Errorf("unsupported platform: %s", key)
	}

	url := "https://github.com/Wilfred/difftastic/releases/latest/download/" + asset

	fmt.Fprintln(os.Stderr, "Installing difftastic...")

	resp, err := http.Get(url)
	if err != nil {
		return "", fmt.Errorf("download difftastic: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("download difftastic: HTTP %d", resp.StatusCode)
	}

	bin, err := extractDifft(resp.Body)
	if err != nil {
		return "", fmt.Errorf("extract difftastic: %w", err)
	}

	if err := os.MkdirAll(filepath.Dir(dest), 0755); err != nil {
		return "", fmt.Errorf("create dir: %w", err)
	}

	if err := os.WriteFile(dest, bin, 0755); err != nil {
		return "", fmt.Errorf("write difft: %w", err)
	}

	return dest, nil
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
