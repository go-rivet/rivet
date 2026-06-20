package fingerprint

import (
	"context"
	"os"
	"path/filepath"
	"regexp"
	"sync"
	"time"

	"github.com/go-rivet/rivet/pkg/rivet/taskfile/ast"
)

// TimestampChecker checks if any source change compared with the generated files,
// using file modifications timestamps.
type TimestampChecker struct {
	tempDir string
	dry     bool
}

func NewTimestampChecker(tempDir string, dry bool) *TimestampChecker {
	return &TimestampChecker{
		tempDir: tempDir,
		dry:     dry,
	}
}

func (checker *TimestampChecker) IsUpToDate(t *ast.Task) (bool, error) {
	src := []*ast.Glob{}
	gen := []*ast.Glob{}
	for _, tr := range t.Transforms {
		src = append(src, tr.Matches...)
		gen = append(gen, tr.Yields...)
	}
	if len(src) == 0 {
		return false, nil
	}

	// Check the timestamp file first.
	timestampModTime, ok, err := checker.checkTimestampFile(t)
	if err != nil {
		return false, err
	} else if !ok {
		return false, nil // Missing timestamp file, task must run.
	}

	// Setup tracking vars.
	var mu sync.Mutex
	generateMaxTime := timestampModTime
	shouldUpdate := false
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Check the generates globs, and collect the max generate time to use
	// when checking the sources globs.
	err = walkDirWithProvider(ctx, t.Dir, gen, newGenerateReadDirProvider(&mu, &generateMaxTime))
	if err != nil {
		return false, err
	}
	if generateMaxTime.IsZero() {
		return false, nil // No files? task must run.
	}

	// Check the sources globs.
	err = walkDirWithProvider(ctx, t.Dir, src, newSourceReadDirProvider(ctx, cancel, &mu, &generateMaxTime, &shouldUpdate))
	if err != nil {
		return false, err
	}
	if shouldUpdate {
		return false, nil // Out of date, task must run.
	}

	// Not Dry? Update the timestamp file.
	if !checker.dry {
		taskTime := time.Now()
		if err := os.Chtimes(checker.timestampFilePath(t), taskTime, taskTime); err != nil {
			return false, err
		}
	}

	return true, nil // Up to date!
}

func (checker *TimestampChecker) Kind() string {
	return "timestamp"
}

func (checker *TimestampChecker) Value(t *ast.Task) (any, error) {
	src := make([]*ast.Glob, 0, len(t.Transforms))
	for _, tr := range t.Transforms {
		src = append(src, tr.Matches...)
	}
	if len(src) == 0 {
		return time.Unix(0, 0), nil
	}

	// Setup tracking vars.
	var mu sync.Mutex
	var sourcesMaxTime time.Time
	ctx := context.Background()

	// Determine the sources max time.
	err := walkDirWithProvider(ctx, t.Dir, src, newMaxTimeReadDirProvider(&mu, &sourcesMaxTime))
	if err != nil {
		return time.Now(), err
	}
	if sourcesMaxTime.IsZero() {
		return time.Unix(0, 0), nil
	}
	return sourcesMaxTime, nil
}

func (*TimestampChecker) OnError(t *ast.Task) error {
	return nil
}

func (checker *TimestampChecker) timestampFilePath(t *ast.Task) string {
	return filepath.Join(checker.tempDir, "timestamp", normalizeFilename(t.Task))
}

var timestampFilenameRegexp = regexp.MustCompile("[^A-z0-9]")

// replaces invalid characters on filenames with "-"
func normalizeFilename(f string) string {
	return timestampFilenameRegexp.ReplaceAllString(f, "-")
}

func (checker *TimestampChecker) checkTimestampFile(t *ast.Task) (time.Time, bool, error) {
	timestampFile := checker.timestampFilePath(t)
	tsInfo, err := os.Stat(timestampFile)
	if err != nil {
		if !checker.dry {
			if err := os.MkdirAll(filepath.Dir(timestampFile), 0o755); err != nil {
				return time.Time{}, false, err
			}
			f, err := os.Create(timestampFile)
			if err != nil {
				return time.Time{}, false, err
			}
			_ = f.Close()
		}
		return time.Time{}, false, nil
	}
	return tsInfo.ModTime(), true, nil
}
