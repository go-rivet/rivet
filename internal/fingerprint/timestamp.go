package fingerprint

import (
	"context"
	"os"
	"path/filepath"
	"regexp"
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
	var matches []*ast.Glob
	var yields []*ast.Glob
	if t.Transform != nil {
		matches = append(matches, t.Transform.Matches...)
		yields = append(yields, t.Transform.Yields...)
		if matchGlob, yieldGlob := t.Transform.SubstToGlob(); matchGlob != nil && yieldGlob != nil {
			matches = append(matches, matchGlob)
			yields = append(yields, yieldGlob)
		}
	}
	if len(matches) == 0 {
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
	generateMaxTime := timestampModTime

	// Check the generates globs, and collect the max generate time to use
	// when checking the sources globs.
	genResults, err := walkDirWithProvider(context.Background(), t.Dir, yields)
	if err != nil {
		return false, err
	}
	maxTime, genOutOfDate, err := evaluateGenerateResults(genResults)
	if err != nil {
		return false, err
	}
	if genOutOfDate {
		return false, nil // Missing/empty generates, task must run.
	}
	if maxTime.After(generateMaxTime) {
		generateMaxTime = maxTime
	}
	if generateMaxTime.IsZero() {
		return false, nil // No files? task must run.
	}

	// Check the sources globs.
	srcResults, err := walkDirWithProvider(context.Background(), t.Dir, matches)
	if err != nil {
		return false, err
	}
	srcOutOfDate, err := evaluateSourceResults(srcResults, generateMaxTime)
	if err != nil {
		return false, err
	}
	if srcOutOfDate {
		return false, nil // Out of date, task must run.
	}

	// Not Dry? Update the timestamp file to the newest verified state (generateMaxTime),
	// rather than time.Now(), so it can't race against a source mutated moments later.
	if !checker.dry {
		if err := os.Chtimes(checker.timestampFilePath(t), generateMaxTime, generateMaxTime); err != nil {
			return false, err
		}
	}

	return true, nil // Up to date!
}

func (checker *TimestampChecker) Kind() string {
	return "timestamp"
}

func (checker *TimestampChecker) Value(t *ast.Task) (any, error) {
	var matches []*ast.Glob
	if t.Transform != nil {
		matches = t.Transform.Matches
	}
	if len(matches) == 0 {
		return time.Unix(0, 0), nil
	}

	ctx := context.Background()

	// Determine the sources max time.
	results, err := walkDirWithProvider(ctx, t.Dir, matches)
	if err != nil {
		return time.Now(), err
	}
	sourcesMaxTime := evaluateMaxTimeResults(results)
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

var timestampFilenameRegexp = regexp.MustCompile(`[^[:alnum:]]`)

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
