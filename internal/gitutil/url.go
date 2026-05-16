package gitutil

import (
	"net/url"
	"strings"
)

func ParseGitURL(rawURL string) (*url.URL, error) {
	// 1. Handle standard local file paths (e.g., /path/to/repo)
	if strings.HasPrefix(rawURL, "/") || strings.HasPrefix(rawURL, "./") || strings.HasPrefix(rawURL, "../") {
		return &url.URL{Scheme: "file", Path: rawURL}, nil
	}

	// 2. Convert SCP-like shorthand (git@github.com:org/repo.git) to valid SSH URLs
	if strings.Contains(rawURL, "@") && strings.Contains(rawURL, ":") && !strings.Contains(rawURL, "://") {
		// Split at the first colon
		parts := strings.SplitN(rawURL, ":", 2)
		// Reconstruct into a standard ssh:// schema URL format
		rawURL = "ssh://" + parts[0] + "/" + parts[1]
	}

	// 3. Use standard Go engine to parse the finalized URL schema
	return url.Parse(rawURL)
}
