package repository

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// FetchRemoteDirectory replaces go-getter. It downloads remote folders/archives or clones Git repositories natively.
func FetchRemoteDirectory(ctx context.Context, srcURL, cacheDir string) (string, error) {
	// 1. Clean the target cache directory completely
	_ = os.RemoveAll(cacheDir)

	// 2. Handle Git Repositories (SSH protocols, git schemas, or URLs targeting a .git extension)
	if strings.HasPrefix(srcURL, "git@") || strings.HasPrefix(srcURL, "ssh://") || strings.Contains(srcURL, ".git") {
		cleanURL := srcURL
		// Strip out go-getter subdirectory indicators (//) or branch queries (?ref=)
		if idx := strings.Index(srcURL, "//"); idx != -1 {
			cleanURL = srcURL[:idx]
		}
		if idx := strings.Index(cleanURL, "?"); idx != -1 {
			cleanURL = cleanURL[:idx]
		}

		// Perform a highly optimized shallow git clone directly via OS shell tools
		cmd := exec.CommandContext(ctx, "git", "clone", "--depth", "1", cleanURL, cacheDir)
		if output, err := cmd.CombinedOutput(); err != nil {
			_ = os.RemoveAll(cacheDir)
			return "", fmt.Errorf("git clone failed (%s): %w", strings.TrimSpace(string(output)), err)
		}
		return cacheDir, nil
	}

	// 3. Handle standard HTTP/HTTPS raw download URLs
	if strings.HasPrefix(srcURL, "http://") || strings.HasPrefix(srcURL, "https://") {
		if err := os.MkdirAll(cacheDir, 0755); err != nil {
			return "", fmt.Errorf("failed to create cache directory: %w", err)
		}

		req, err := http.NewRequestWithContext(ctx, "GET", srcURL, nil)
		if err != nil {
			return "", err
		}

		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			_ = os.RemoveAll(cacheDir)
			return "", fmt.Errorf("http request failed: %w", err)
		}
		defer func() {
			_ = resp.Body.Close()
		}()
		if resp.StatusCode != http.StatusOK {
			_ = os.RemoveAll(cacheDir)
			return "", fmt.Errorf("unexpected server response status: %s", resp.Status)
		}

		// Save the network stream payload to disk natively
		tmpFile := filepath.Join(cacheDir, "download.tmp")
		out, err := os.Create(tmpFile)
		if err != nil {
			return "", err
		}
		defer func() {
			_ = out.Close()
		}()

		if _, err = io.Copy(out, resp.Body); err != nil {
			_ = os.RemoveAll(cacheDir)
			return "", fmt.Errorf("failed to write download stream: %w", err)
		}

		return cacheDir, nil
	}

	return "", fmt.Errorf("unsupported remote repository protocol schema string: %s", srcURL)
}
