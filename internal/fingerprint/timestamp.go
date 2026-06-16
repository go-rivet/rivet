package fingerprint

import (
	"context"
	"os"
	"path/filepath"
	"time"
	"unsafe"

	"github.com/go-rivet/rivet/pkg/rivet/taskfile/ast"
	"golang.org/x/sync/errgroup"
)

// fileEntry unifies the internal representation across functions to satisfy the Go compiler.
type fileEntry struct {
	info os.FileInfo
	keep bool
}

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

// IsUpToDate implements the Checker interface
func (checker *TimestampChecker) IsUpToDate(t *ast.Task) (bool, error) {
	// Pre-allocate slice capacities to avoid intermediate grow reallocations
	src := make([]*ast.Glob, 0, len(t.Transforms))
	gen := make([]*ast.Glob, 0, len(t.Transforms))
	for _, tr := range t.Transforms {
		src = append(src, tr.Matches...)
		gen = append(gen, tr.Yields...)
	}

	if len(src) == 0 {
		return false, nil
	}

	// 1. Invert Check Sequence: Evaluate tracking file existence BEFORE walking thousands of files.
	// If the timestamp file is entirely missing, the task is out of date; skip the 20,000 file scans entirely.
	timestampFile := checker.timestampFilePath(t)
	var timestampModTime time.Time
	tsInfo, err := os.Stat(timestampFile)
	if err != nil {
		if !checker.dry {
			if err := os.MkdirAll(filepath.Dir(timestampFile), 0o755); err != nil {
				return false, err
			}
			f, err := os.Create(timestampFile)
			if err != nil {
				return false, err
			}
			_ = f.Close()
		}
		return false, nil // Missing tracking file forces immediate execution execution path
	}
	timestampModTime = tsInfo.ModTime()

	// Pre-verify generator metadata configurations before checking the disk
	hasValidGen := false
	if len(gen) > 0 {
		for _, g := range gen {
			if !g.Negate {
				hasValidGen = true
				break
			}
		}
	}

	// 2. Asynchronous Short-Circuiting: Scan sources and generates concurrently.
	// Context cancellation lets one worker abort the other worker if an invalid state is encountered.
	g, ctx := errgroup.WithContext(context.Background())

	var sources []string
	var sourceInfos []os.FileInfo
	g.Go(func() error {
		var err error
		sources, sourceInfos, err = GlobsWithInfo(ctx, t.Dir, src, true)
		return err
	})

	var generates []string
	var generateInfos []os.FileInfo
	g.Go(func() error {
		var err error
		generates, generateInfos, err = GlobsWithInfo(ctx, t.Dir, gen, true)
		return err
	})

	if err := g.Wait(); err != nil {
		// Context cancellation returns a clean skip signal instead of a hard application error
		if ctx.Err() != nil {
			return false, nil
		}
		return false, err
	}

	if len(sources) == 0 {
		return false, nil
	}

	// If explicit yields were expected but none matched physically on disk, it's out of date
	if hasValidGen && len(generates) == 0 {
		return false, nil
	}

	taskTime := time.Now()

	// Find max modification time among generated files using already-allocated slice metadata
	var generateMaxTime time.Time
	for _, info := range generateInfos {
		if info.ModTime().After(generateMaxTime) {
			generateMaxTime = info.ModTime()
		}
	}

	// Compare with the pre-fetched tracking file timestamp
	if timestampModTime.After(generateMaxTime) {
		generateMaxTime = timestampModTime
	}

	if generateMaxTime.IsZero() {
		return false, nil
	}

	// Early Short-Circuiting: Evaluate source file times immediately using cached memory metadata
	shouldUpdate := false
	for _, info := range sourceInfos {
		if info.ModTime().After(generateMaxTime) {
			shouldUpdate = true
			break // Instantly stop checking the remaining thousands of files
		}
	}

	if !checker.dry {
		if err := os.Chtimes(timestampFile, taskTime, taskTime); err != nil {
			return false, err
		}
	}

	return !shouldUpdate, nil
}

func (checker *TimestampChecker) Kind() string {
	return "timestamp"
}

// Value implements the Checker Interface
func (checker *TimestampChecker) Value(t *ast.Task) (any, error) {
	src := make([]*ast.Glob, 0, len(t.Transforms))
	for _, tr := range t.Transforms {
		src = append(src, tr.Matches...)
	}

	_, sourceInfos, err := GlobsWithInfo(context.Background(), t.Dir, src, true)
	if err != nil {
		return time.Now(), err
	}

	var sourcesMaxTime time.Time
	for _, info := range sourceInfos {
		if info.ModTime().After(sourcesMaxTime) {
			sourcesMaxTime = info.ModTime()
		}
	}

	if sourcesMaxTime.IsZero() {
		return time.Unix(0, 0), nil
	}

	return sourcesMaxTime, nil
}

// OnError implements the Checker interface
func (*TimestampChecker) OnError(t *ast.Task) error {
	return nil
}

func (checker *TimestampChecker) timestampFilePath(t *ast.Task) string {
	return filepath.Join(checker.tempDir, "timestamp", normalizeFilename(t.Task))
}

// 3. Optimize normalizeFilename: Read-only scan checks characters directly.
// Allocates memory ONLY if the string contains invalid characters that must be rewritten.
func normalizeFilename(f string) string {
	if f == "" {
		return ""
	}

	// Scan step: Check if anything needs modification before allocating byte allocations
	needsChange := false
	for i := 0; i < len(f); i++ {
		c := f[i]
		if !((c >= 'A' && c <= 'Z') || (c >= 'a' && c <= 'z') || (c >= '0' && c <= '9')) {
			needsChange = true
			break
		}
	}

	// Fast Path: String is already normalized; return instantly with zero allocations.
	if !needsChange {
		return f
	}

	// Slow Path: Allocate a mutable segment only when invalid characters are confirmed.
	b := []byte(f)
	for i, c := range b {
		if !((c >= 'A' && c <= 'Z') || (c >= 'a' && c <= 'z') || (c >= '0' && c <= '9')) {
			b[i] = '-'
		}
	}

	// Safe zero-allocation conversion from []byte to string
	return unsafe.String(&b[0], len(b))
}
