package fingerprint

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
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

type readDirProvider func(negate bool) func(string) ([]fs.DirEntry, error)

func escape(s string) string {
	if runtime.GOOS == "windows" {
		s = strings.ReplaceAll(s, `\`, `/`)
	}
	quoted, err := syntax.Quote(s, syntax.LangPOSIX)
	if err != nil {
		return s
	}
	return quoted
}

func walkDirWithProvider(ctx context.Context, dir string, globs []*ast.Glob, provider readDirProvider) error {
	if len(globs) == 0 {
		return nil
	}

	g, ctx := errgroup.WithContext(ctx)

	for _, gPattern := range globs {
		gPattern := gPattern

		g.Go(func() error {
			if err := ctx.Err(); err != nil {
				return nil
			}
			fullPath := filepathext.SmartJoin(dir, gPattern.Glob)

			if pattern.HasMeta(gPattern.Glob, 0) {
				// Glob pattern.
				p := syntax.NewParser()
				var words []*syntax.Word
				for w, err := range p.WordsSeq(strings.NewReader(escape(fullPath))) {
					if err != nil {
						return err
					}
					words = append(words, w)
				}
				cfg := &expand.Config{
					Env:      expand.FuncEnviron(os.Getenv),
					ReadDir2: provider(gPattern.Negate),
					GlobStar: true,
					NullGlob: true,
				}
				_, err := expand.Fields(cfg, words...)
				if err != nil && !errors.Is(err, context.Canceled) {
					return err
				}
				return nil
			} else {
				// Non-glob pattern (i.e. specific file path). Directly run the provider.
				interceptor := provider(gPattern.Negate)
				cleanPath := filepath.Clean(fullPath)
				parentDir := filepath.Dir(cleanPath)
				baseName := filepath.Base(cleanPath)

				// Synthesize a synthetic directory listing containing just this specific file.
				entries, err := os.ReadDir(parentDir)
				if err != nil {
					if errors.Is(err, fs.ErrNotExist) {
						return nil
					}
					return err
				}
				var singleEntry []fs.DirEntry
				for _, entry := range entries {
					if entry.Name() == baseName {
						singleEntry = []fs.DirEntry{entry}
						break
					}
				}

				// Execute the interceptor manually on the file's parent directory.
				_, err = func(dirPath string) ([]fs.DirEntry, error) {
					if gPattern.Negate {
						return singleEntry, nil
					}
					// Replicate the target core logic of the interceptor loop manually
					if len(singleEntry) > 0 && !singleEntry[0].IsDir() {
						_, err := interceptor(cleanPath)
						if err != nil && !errors.Is(err, context.Canceled) {
							if strings.Contains(err.Error(), "not a directory") || strings.Contains(err.Error(), "invalid parameter") {
								return nil, nil
							}
							return nil, err
						}
					}
					return singleEntry, nil
				}(parentDir)

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

func newGenerateReadDirProvider(mu *sync.Mutex, maxTime *time.Time) readDirProvider {
	return func(negate bool) func(string) ([]fs.DirEntry, error) {
		return func(dirPath string) ([]fs.DirEntry, error) {
			entries, err := os.ReadDir(dirPath)
			if err != nil {
				return nil, err
			}
			if negate {
				return entries, nil
			}

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

func newSourceReadDirProvider(ctx context.Context, cancel context.CancelFunc, mu *sync.Mutex, generateMaxTime *time.Time, outOfDate *bool) readDirProvider {
	return func(negate bool) func(string) ([]fs.DirEntry, error) {
		return func(dirPath string) ([]fs.DirEntry, error) {
			if err := ctx.Err(); err != nil {
				return nil, err
			}

			entries, err := os.ReadDir(dirPath)
			if err != nil {
				return nil, err
			}
			if negate {
				return entries, nil
			}

			for _, entry := range entries {
				if entry.IsDir() {
					continue
				}
				if info, err := entry.Info(); err == nil {
					modTime := info.ModTime()
					mu.Lock()
					threshold := *generateMaxTime
					mu.Unlock()
					if modTime.After(threshold) {
						mu.Lock()
						*outOfDate = true
						mu.Unlock()
						cancel() // Abort all running directories concurrently
						return nil, context.Canceled
					}
				}
			}
			return entries, nil
		}
	}
}

func newMaxTimeReadDirProvider(mu *sync.Mutex, maxTime *time.Time) readDirProvider {
	return func(negate bool) func(string) ([]fs.DirEntry, error) {
		return func(dirPath string) ([]fs.DirEntry, error) {
			entries, err := os.ReadDir(dirPath)
			if err != nil {
				return nil, err
			}
			if negate {
				return entries, nil
			}

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
