package fingerprint

import (
	"context"
	"os"
	"path/filepath"
	"sort"

	"github.com/go-rivet/rivet/internal/execext"
	"github.com/go-rivet/rivet/internal/filepathext"
	"github.com/go-rivet/rivet/pkg/rivet/taskfile/ast"
)

func GlobsWithInfo(ctx context.Context, dir string, globs []*ast.Glob, skipSort bool) ([]string, []os.FileInfo, error) {
	if len(globs) == 0 {
		return nil, nil, nil
	}

	// Pre-size the map to prevent continuous map resizing overhead at scale
	resultMap := make(map[string]fileEntry, 100)

	for _, g := range globs {
		// 1. Context Check: Intercept and abort processing between glob expansions if another goroutine cancelled the run.
		if err := ctx.Err(); err != nil {
			return nil, nil, err
		}

		fullPattern := filepathext.SmartJoin(dir, g.Glob)

		// Expand the path/glob string using internal toolchain configurations
		fs, err := execext.ExpandFields(fullPattern)
		if err != nil {
			return nil, nil, err
		}

		for _, f := range fs {
			// 2. High-Frequency Context Check: At a scale of 2,000–20,000 files, checking the context
			// before hitting the disk ensures we stop immediately if the task state becomes invalid.
			if err := ctx.Err(); err != nil {
				return nil, nil, err
			}

			info, err := os.Stat(f)
			if err != nil {
				continue // Skip files that can't be statted (permissions, broken links)
			}
			if info.IsDir() {
				continue // Skip directories immediately
			}

			normPath := filepath.ToSlash(f)

			resultMap[normPath] = fileEntry{
				info: info,
				keep: !g.Negate,
			}
		}
	}

	return collectKeysAndMetadata(resultMap, skipSort)
}

func Globs(dir string, globs []*ast.Glob) ([]string, error) {
	paths, _, err := GlobsWithInfo(context.Background(), dir, globs, false)
	return paths, err
}

func collectKeysAndMetadata(m map[string]fileEntry, skipSort bool) ([]string, []os.FileInfo, error) {
	// Pre-allocate exact capacities for 2,000 - 20,000 items to avoid GC thrashing
	validCount := 0
	for _, entry := range m {
		if entry.keep {
			validCount++
		}
	}

	if validCount == 0 {
		return nil, nil, nil
	}

	keys := make([]string, 0, validCount)

	// Fast Path: If sorting is skipped, we can extract the metadata directly
	// in a single map traversal loop without ever touching sort.Strings.
	if skipSort {
		infos := make([]os.FileInfo, 0, validCount)
		for k, entry := range m {
			if entry.keep {
				keys = append(keys, k)
				infos = append(infos, entry.info)
			}
		}
		return keys, infos, nil
	}

	// Slow Path: Collect, sort, and reconstruct slices sequentially for legacy compatibility
	for k, entry := range m {
		if entry.keep {
			keys = append(keys, k)
		}
	}

	sort.Strings(keys)

	// Build the matching metadata slice mapped directly to the sorted paths
	infos := make([]os.FileInfo, len(keys))
	for i, path := range keys {
		infos[i] = m[path].info
	}

	return keys, infos, nil
}
