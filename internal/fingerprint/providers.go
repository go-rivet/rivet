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

// Functional Closure to encapsulate state into the mcdancc/sh ReadDir2 interface (which is itself stateless). This
// makes it possible to directly evaluate state (maxTime, outOfDate) without needing to accumulate intermediate objects.

var ErrNoSourcesMatched = errors.New("fingerprint: source glob pattern matched zero files")

// readDirProvider defines a higher-order interceptor factory type.
type readDirProvider func(negate bool) func(string, func(string) ([]fs.DirEntry, error)) ([]fs.DirEntry, error)

// walkDirWithProvider processes an array of task-scoped patterns concurrently.
func walkDirWithProvider(ctx context.Context, dir string, globs []*ast.Glob, provider readDirProvider) error {
	if len(globs) == 0 {
		return nil
	}
	g, ctx := errgroup.WithContext(ctx)

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

			// Process each pattern.
			if pattern.HasMeta(gPattern.Glob, 0) {
				// Glob pattern, call the interceptor indirectly (via ReadDir2).
				cfg := &expand.Config{
					Env: expand.FuncEnviron(os.Getenv),
					ReadDir2: func(dirPath string) ([]fs.DirEntry, error) {
						return provider(gPattern.Negate)(dirPath, os.ReadDir)
					},
					GlobStar: true,
					NullGlob: true,
				}
				_, err := expand.Fields(cfg, words...)
				if err != nil && !errors.Is(err, context.Canceled) {
					return err
				}
				return nil
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
				parentDir := filepath.Dir(cleanPath)
				fi, err := os.Stat(cleanPath)
				if err != nil && !errors.Is(err, fs.ErrNotExist) {
					return err
				}

				// Call the interceptor directly with a mock/synthetic ReadDir.
				interceptor := provider(gPattern.Negate)
				mockReadDir := func(dirPath string) ([]fs.DirEntry, error) {
					if errors.Is(err, fs.ErrNotExist) {
						return []fs.DirEntry{}, nil
					}
					return []fs.DirEntry{fs.FileInfoToDirEntry(fi)}, nil
				}
				_, err = interceptor(parentDir, mockReadDir)
				if err != nil && !errors.Is(err, context.Canceled) {
					return err
				}
				return nil
			}
		})
	}

	if err := g.Wait(); err != nil && !errors.Is(err, context.Canceled) {
		return err
	}
	return nil
}

// newGenerateReadDirProvider initializes an output tracker interceptor.
func newGenerateReadDirProvider(ctx context.Context, cancel context.CancelFunc, mu *sync.Mutex, maxTime *time.Time, outOfDate *bool) readDirProvider {
	return func(negate bool) func(string, func(string) ([]fs.DirEntry, error)) ([]fs.DirEntry, error) {
		return func(dirPath string, readDir func(string) ([]fs.DirEntry, error)) ([]fs.DirEntry, error) {
			// Check if generates item exists.
			entries, err := readDir(dirPath)
			if err != nil {
				if errors.Is(err, fs.ErrNotExist) {
					mu.Lock()
					*outOfDate = true
					mu.Unlock()
					cancel()
				}
				return nil, err
			}
			if negate {
				return entries, nil
			}

			// Check that there is a generates item (i.e. something matched).
			if len(entries) == 0 {
				mu.Lock()
				*outOfDate = true
				mu.Unlock()
				cancel()
				return entries, context.Canceled
			}

			// Determine if any entires are out of date.
			var localMax time.Time
			hasFiles := false
			for _, entry := range entries {
				if entry.IsDir() {
					continue
				}
				if info, err := entry.Info(); err == nil {
					hasFiles = true
					modTime := info.ModTime()
					if modTime.After(localMax) {
						localMax = modTime
					}
				} else {
					mu.Lock()
					*outOfDate = true
					mu.Unlock()
					cancel()
					return nil, err
				}
			}
			if !hasFiles {
				// No files (in this dir) matched.
				mu.Lock()
				*outOfDate = true
				mu.Unlock()
				cancel()
				return entries, context.Canceled
			}

			// Update the maxtime (based on this dir).
			mu.Lock()
			if localMax.After(*maxTime) {
				*maxTime = localMax
			}
			mu.Unlock()

			return entries, nil
		}
	}
}

// newSourceReadDirProvider initializes an source tracker interceptor.
func newSourceReadDirProvider(ctx context.Context, cancel context.CancelFunc, mu *sync.Mutex, generateMaxTime *time.Time, outOfDate *bool) readDirProvider {
	return func(negate bool) func(string, func(string) ([]fs.DirEntry, error)) ([]fs.DirEntry, error) {
		return func(dirPath string, readDir func(string) ([]fs.DirEntry, error)) ([]fs.DirEntry, error) {
			if err := ctx.Err(); err != nil {
				return nil, err
			}
			entries, err := readDir(dirPath)
			if err != nil {
				return nil, err
			}
			if negate {
				return entries, nil
			}
			if len(entries) == 0 {
				// This should really be an error condition, but it fails lots of tests.
				//cancel()
				//return nil, ErrNoSourcesMatched
			}

			// Determine if any entires are out of date.
			mu.Lock()
			threshold := *generateMaxTime
			mu.Unlock()
			for _, entry := range entries {
				if entry.IsDir() {
					continue
				}
				if info, err := entry.Info(); err == nil {
					modTime := info.ModTime()
					if modTime.After(threshold) {
						mu.Lock()
						*outOfDate = true
						mu.Unlock()
						cancel()
						return nil, context.Canceled
					}
				}
			}
			return entries, nil
		}
	}
}

// newMaxTimeReadDirProvider initializes a metadata tracking interceptor.
func newMaxTimeReadDirProvider(mu *sync.Mutex, maxTime *time.Time) readDirProvider {
	return func(negate bool) func(string, func(string) ([]fs.DirEntry, error)) ([]fs.DirEntry, error) {
		return func(dirPath string, readDir func(string) ([]fs.DirEntry, error)) ([]fs.DirEntry, error) {
			entries, err := readDir(dirPath)
			if err != nil {
				return nil, err
			}
			if negate {
				return entries, nil
			}

			// Determine the latest modification time.
			for _, entry := range entries {
				if entry.IsDir() {
					continue
				}
				if info, err := entry.Info(); err == nil {
					modTime := info.ModTime()
					mu.Lock()
					if modTime.After(*maxTime) {
						*maxTime = modTime
					}
					mu.Unlock()
				}
			}
			return entries, nil
		}
	}
}
