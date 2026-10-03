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

type TimestampSourceSnapshot struct {
	task    string
	dir     string
	globs   []globSnapshotKey
	results []dirReadResult
}

type globSnapshotKey struct {
	glob   string
	negate bool
}

func NewTimestampChecker(tempDir string, dry bool) *TimestampChecker {
	return &TimestampChecker{
		tempDir: tempDir,
		dry:     dry,
	}
}

func (checker *TimestampChecker) IsUpToDate(t *ast.Task) (bool, error) {
	return checker.IsUpToDateWithSnapshot(t, nil)
}

func (checker *TimestampChecker) IsUpToDateWithSnapshot(t *ast.Task, snapshot *TimestampSourceSnapshot) (bool, error) {
	matches, yields := timestampPatterns(t)
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

	// Generated outputs are distinct from the source scan and must be checked here.
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

	var srcResults []dirReadResult
	if snapshot != nil && snapshot.matches(t, matches) {
		srcResults = snapshot.results
	} else {
		srcResults, err = walkDirWithProvider(context.Background(), t.Dir, matches)
		if err != nil {
			return false, err
		}
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

func timestampPatterns(t *ast.Task) (matches, yields []*ast.Glob) {
	if t.Transform != nil {
		matches = append(matches, t.Transform.Matches...)
		yields = append(yields, t.Transform.Yields...)
		if matchGlob, yieldGlob := t.Transform.SubstToGlob(); matchGlob != nil && yieldGlob != nil {
			matches = append(matches, matchGlob)
			yields = append(yields, yieldGlob)
		}
	}
	return matches, yields
}

func (checker *TimestampChecker) Kind() string {
	return "timestamp"
}

func (checker *TimestampChecker) Value(t *ast.Task) (any, error) {
	value, _, err := checker.ValueWithSnapshot(t)
	return value, err
}

func (checker *TimestampChecker) ValueWithSnapshot(t *ast.Task) (any, *TimestampSourceSnapshot, error) {
	var matches []*ast.Glob
	if t.Transform != nil {
		matches = t.Transform.Matches
	}
	if len(matches) == 0 {
		return time.Unix(0, 0), nil, nil
	}

	sourceGlobs, _ := timestampPatterns(t)
	results, err := walkDirWithProvider(context.Background(), t.Dir, sourceGlobs)
	if err != nil {
		return time.Now(), nil, err
	}
	sourceSet := make(map[globSnapshotKey]struct{}, len(matches))
	for _, g := range matches {
		sourceSet[globSnapshotKey{glob: g.Glob, negate: g.Negate}] = struct{}{}
	}
	valueResults := make([]dirReadResult, 0, len(matches))
	for _, result := range results {
		if _, ok := sourceSet[globSnapshotKey{glob: result.glob, negate: result.negate}]; ok {
			valueResults = append(valueResults, result)
		}
	}
	sourcesMaxTime := evaluateMaxTimeResults(valueResults)
	snapshot := &TimestampSourceSnapshot{
		task:    t.Task,
		dir:     t.Dir,
		globs:   snapshotGlobKeys(sourceGlobs),
		results: results,
	}
	if sourcesMaxTime.IsZero() {
		return time.Unix(0, 0), snapshot, nil
	}
	return sourcesMaxTime, snapshot, nil
}

func snapshotGlobKeys(globs []*ast.Glob) []globSnapshotKey {
	keys := make([]globSnapshotKey, len(globs))
	for i, g := range globs {
		keys[i] = globSnapshotKey{glob: g.Glob, negate: g.Negate}
	}
	return keys
}

func (snapshot *TimestampSourceSnapshot) matches(t *ast.Task, globs []*ast.Glob) bool {
	if snapshot == nil || snapshot.task != t.Task || snapshot.dir != t.Dir || len(snapshot.globs) != len(globs) {
		return false
	}
	for i, g := range globs {
		if snapshot.globs[i] != (globSnapshotKey{glob: g.Glob, negate: g.Negate}) {
			return false
		}
	}
	return true
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
