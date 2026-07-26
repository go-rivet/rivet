package fingerprint

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/go-rivet/rivet/internal/filepathext"
	"github.com/go-rivet/rivet/pkg/rivet/taskfile/ast"
	"golang.org/x/sync/errgroup"
	"mvdan.cc/sh/v3/expand"
	"mvdan.cc/sh/v3/pattern"
	"mvdan.cc/sh/v3/syntax"
)

var ErrNoSourcesMatched = errors.New("fingerprint: source glob pattern matched zero files")

// dirReadResult holds the fs.DirEntry values that a single task-scoped glob pattern
// matched. All differentiating logic (out-of-date checks, max-time tracking, etc.)
// is applied afterward by inspecting the collected results.
type dirReadResult struct {
	negate  bool
	entries []fs.DirEntry
}

// walkDirWithProvider evaluates an array of task-scoped patterns concurrently. Each
// pattern is expanded/glob-matched via mvdan.cc/sh/expand, and the resulting matched
// files are resolved into fs.DirEntry values for the caller to evaluate afterward.
func walkDirWithProvider(ctx context.Context, dir string, globs []*ast.Glob) ([]dirReadResult, error) {
	if len(globs) == 0 {
		return nil, nil
	}
	g, ctx := errgroup.WithContext(ctx)

	var mu sync.Mutex
	var results []dirReadResult

	for _, gPattern := range globs {
		g.Go(func() error {
			if err := ctx.Err(); err != nil {
				return nil
			}
			fullPath := filepathext.SmartJoin(dir, gPattern.Glob)

			// Parse variables first to resolve non-glob expansions (e.g. $HOME/file.txt)
			p := syntax.NewParser()
			var words []*syntax.Word
			for w, err := range p.WordsSeq(strings.NewReader(fullPath)) {
				if err != nil {
					return err
				}
				words = append(words, w)
			}

			var entries []fs.DirEntry
			if pattern.HasMeta(gPattern.Glob, 0) {
				// Glob pattern: expand.Fields applies the actual shell glob matching
				// and returns the matched paths, which we resolve into DirEntry values.
				cfg := &expand.Config{
					Env: expand.FuncEnviron(os.Getenv),
					ReadDir2: func(dirPath string) ([]fs.DirEntry, error) {
						return os.ReadDir(dirPath)
					},
					GlobStar: true,
					NullGlob: true,
				}
				matchedPaths, err := expand.Fields(cfg, words...)
				if err != nil {
					return err
				}
				entries, err = statMatchedPaths(matchedPaths)
				if err != nil {
					return err
				}
			} else {
				// Non-glob pattern (i.e. single file/directory).
				var expandedWords []string
				cfg := &expand.Config{
					Env: expand.FuncEnviron(os.Getenv),
				}
				for _, w := range words {
					expr, err := expand.Literal(cfg, w)
					if err != nil {
						return err
					}
					expandedWords = append(expandedWords, expr)
				}
				expandedPath := strings.Join(expandedWords, " ")

				cleanPath := filepath.Clean(expandedPath)
				fi, err := os.Stat(cleanPath)
				if err != nil && !errors.Is(err, fs.ErrNotExist) {
					return err
				}
				if err == nil {
					entries = []fs.DirEntry{fs.FileInfoToDirEntry(fi)}
				}
			}

			mu.Lock()
			results = append(results, dirReadResult{negate: gPattern.Negate, entries: entries})
			mu.Unlock()
			return nil
		})
	}

	if err := g.Wait(); err != nil {
		return nil, err
	}
	return results, nil
}

// statMatchedPaths resolves the glob-matched paths returned by expand.Fields into
// their corresponding fs.DirEntry values.
func statMatchedPaths(paths []string) ([]fs.DirEntry, error) {
	entries := make([]fs.DirEntry, 0, len(paths))
	for _, path := range paths {
		fi, err := os.Lstat(path)
		if err != nil {
			if errors.Is(err, fs.ErrNotExist) {
				continue
			}
			return nil, err
		}
		entries = append(entries, fs.FileInfoToDirEntry(fi))
	}
	return entries, nil
}

// evaluateGenerateResults determines the max modification time across generated
// outputs and whether they are out of date (missing, or a matched dir has no files).
func evaluateGenerateResults(results []dirReadResult) (maxTime time.Time, outOfDate bool, err error) {
	for _, r := range results {
		if r.negate {
			continue
		}
		if len(r.entries) == 0 {
			return maxTime, true, nil
		}
		hasFiles := false
		for _, entry := range r.entries {
			if entry.IsDir() {
				continue
			}
			info, err := entry.Info()
			if err != nil {
				return maxTime, true, err
			}
			hasFiles = true
			if modTime := info.ModTime(); modTime.After(maxTime) {
				maxTime = modTime
			}
		}
		if !hasFiles {
			return maxTime, true, nil
		}
	}
	return maxTime, false, nil
}

// evaluateSourceResults reports whether any collected source file is newer than threshold.
func evaluateSourceResults(results []dirReadResult, threshold time.Time) (outOfDate bool, err error) {
	for _, r := range results {
		if r.negate {
			continue
		}
		for _, entry := range r.entries {
			if entry.IsDir() {
				continue
			}
			info, err := entry.Info()
			if err != nil {
				return false, err
			}
			if info.ModTime().After(threshold) {
				return true, nil
			}
		}
	}
	return false, nil
}

// evaluateMaxTimeResults determines the latest modification time across all collected entries.
func evaluateMaxTimeResults(results []dirReadResult) time.Time {
	var maxTime time.Time
	for _, r := range results {
		if r.negate {
			continue
		}
		for _, entry := range r.entries {
			if entry.IsDir() {
				continue
			}
			if info, err := entry.Info(); err == nil {
				if modTime := info.ModTime(); modTime.After(maxTime) {
					maxTime = modTime
				}
			}
		}
	}
	return maxTime
}
