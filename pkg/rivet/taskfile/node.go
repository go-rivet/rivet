package taskfile

import (
	"context"
	"strings"
	"time"

	"github.com/go-rivet/rivet/internal/fsext"
	"github.com/go-rivet/rivet/internal/gitutil"
)

type Node interface {
	Read() ([]byte, error)
	Parent() Node
	Location() string
	Dir() string
	Checksum() string
	Verify(checksum string) bool
	ResolveEntrypoint(entrypoint string) (string, error)
	ResolveDir(dir string) (string, error)
}

type RemoteNode interface {
	Node
	ReadContext(ctx context.Context) ([]byte, error)
	CacheKey() string
}

func NewRootNode(
	entrypoint string,
	dir string,
	insecure bool,
	timeout time.Duration,
	opts ...NodeOption,
) (Node, error) {
	dir = fsext.DefaultDir(entrypoint, dir)
	// If the entrypoint is "-", we read from stdin
	if entrypoint == "-" {
		return NewStdinNode(dir)
	}
	return NewNode(entrypoint, dir, insecure, opts...)
}

func NewNode(
	entrypoint string,
	dir string,
	insecure bool,
	opts ...NodeOption,
) (Node, error) {
	var node Node
	var err error

	scheme, err := getScheme(entrypoint)
	if err != nil {
		return nil, err
	}

	switch scheme {
	case "git":
		node, err = NewGitNode(entrypoint, dir, insecure, opts...)
	case "http", "https":
		node, err = NewHTTPNode(entrypoint, dir, insecure, opts...)
	default:
		node, err = NewFileNode(entrypoint, dir, opts...)
	}

	return node, err
}

func isRemoteEntrypoint(entrypoint string) bool {
	scheme, _ := getScheme(entrypoint)
	switch scheme {
	case "git", "http", "https":
		return true
	default:
		return false
	}
}

func getScheme(uri string) (string, error) {
	u, err := gitutil.ParseGitURL(uri)
	if u == nil {
		return "", err
	}

	if strings.HasSuffix(strings.Split(u.Path, "//")[0], ".git") && (u.Scheme == "git" || u.Scheme == "ssh" || u.Scheme == "https" || u.Scheme == "http") {
		return "git", nil
	}

	if before, _, ok := strings.Cut(uri, "://"); ok {
		return before, nil
	}

	return "", nil
}
