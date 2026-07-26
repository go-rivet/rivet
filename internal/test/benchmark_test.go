package test

import (
	"bytes"
	"context"
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/go-rivet/rivet/pkg/rivet"
	"github.com/go-rivet/rivet/pkg/rlog"
	"github.com/stretchr/testify/require"
)

const (
	issue2853ManySmallYAMLFileCount = 20_000
	issue2853SmallYAMLFileSize      = 5
	issue2853FewLargeYAMLFileCount  = 4
	issue2853LargeYAMLFileSize      = 128 * 1024 * 1024
)

type SyncBuffer struct {
	buf bytes.Buffer
	mu  sync.Mutex
}

func (sb *SyncBuffer) Write(p []byte) (n int, err error) {
	sb.mu.Lock()
	defer sb.mu.Unlock()
	return sb.buf.Write(p)
}
func SetupTestLogger(t *testing.B, verbose, silent bool) (context.Context, *SyncBuffer, *slog.LevelVar) {
	t.Helper()

	var buffer SyncBuffer
	levelVar := new(slog.LevelVar)
	levelVar.Set(slog.LevelError)
	logOpts := &slog.HandlerOptions{Level: levelVar}
	if silent {
		levelVar.Set(slog.LevelError)
	} else if verbose {
		levelVar.Set(slog.LevelError)
	}
	logHandler := rlog.NewCliHandler(&buffer, &buffer, false, logOpts)
	ctx := rlog.WithContext(t.Context(), logHandler)
	return ctx, &buffer, levelVar
}

func BenchmarkIssue2853ManySmallSparseYAMLFiles(b *testing.B) {
	dir := b.TempDir()
	createIssue2853Fixture(b, dir, issue2853ManySmallYAMLFileCount, issue2853SmallYAMLFileSize)

	benchmarkIssue2853Modes(b, dir, issue2853ManySmallYAMLFileCount, issue2853SmallYAMLFileSize)
}

func BenchmarkIssue2853FewLargeSparseYAMLFiles(b *testing.B) {
	dir := b.TempDir()
	createIssue2853Fixture(b, dir, issue2853FewLargeYAMLFileCount, issue2853LargeYAMLFileSize)

	benchmarkIssue2853Modes(b, dir, issue2853FewLargeYAMLFileCount, issue2853LargeYAMLFileSize)
}

func benchmarkIssue2853Modes(b *testing.B, dir string, fileCount int, fileSize int64) {
	b.Helper()

	for _, mode := range []struct {
		name        string
		task        string
		expectCache bool
		nativeMTime bool
	}{
		{name: "timestamp", task: "timestamp-yaml", expectCache: true},
		{name: "native-mtime", nativeMTime: true},
		{name: "none", task: "uncached-yaml"},
	} {
		b.Run(mode.name, func(b *testing.B) {
			if mode.nativeMTime {
				benchmarkIssue2853NativeMTime(b, dir, fileCount, fileSize)
				return
			}
			benchmarkIssue2853Task(b, dir, mode.task, mode.expectCache, fileCount, fileSize)
		})
	}
}

func benchmarkIssue2853Task(
	b *testing.B,
	dir string,
	taskName string,
	expectCache bool,
	fileCount int,
	fileSize int64,
) {
	b.Helper()
	ctx, buffer, levelVar := SetupTestLogger(b, false, false)
	tempDir := rivet.TempDir{
		Remote:      filepath.Join(dir, ".task"),
		Fingerprint: filepath.Join(dir, ".task"),
	}

	if expectCache {
		e := rivet.NewExecutor(
			rivet.WithLevelVar(levelVar),
			rivet.WithDir(dir),
			rivet.WithStdout(buffer),
			rivet.WithStderr(buffer),
			rivet.WithTempDir(tempDir),
		)
		require.NoError(b, e.Setup(ctx))
		require.NoError(b, e.Run(b.Context(), &rivet.Call{Task: taskName}))
	}

	b.ReportAllocs()
	sourceBytes := int64(fileCount) * fileSize
	if expectCache {
		b.SetBytes(sourceBytes)
	}
	b.ResetTimer()
	for range b.N {
		ctx, buffer, levelVar := SetupTestLogger(b, false, false)

		e := rivet.NewExecutor(
			rivet.WithLevelVar(levelVar),
			rivet.WithDir(dir),
			rivet.WithStdout(buffer),
			rivet.WithStderr(buffer),
			rivet.WithTempDir(tempDir),
		)
		require.NoError(b, e.Setup(ctx))
		require.NoError(b, e.Run(b.Context(), &rivet.Call{Task: taskName}))

	}
	if expectCache {
		b.ReportMetric(float64(fileCount), "source_files/op")
		b.ReportMetric(float64(sourceBytes)/(1024*1024), "source_MiB/op")
	}
}

func benchmarkIssue2853NativeMTime(b *testing.B, dir string, fileCount int, fileSize int64) {
	b.Helper()

	output := filepath.Join(dir, "out", "native-mtime.txt")
	require.NoError(b, os.WriteFile(output, []byte("ok"), 0o644))
	outputTime := time.Now().Add(time.Second)
	require.NoError(b, os.Chtimes(output, outputTime, outputTime))

	sourceRoot := filepath.Join(dir, "path", "to", "folder")
	sourceBytes := int64(fileCount) * fileSize

	b.ReportAllocs()
	b.SetBytes(sourceBytes)
	b.ResetTimer()
	for range b.N {
		outputInfo, err := os.Stat(output)
		require.NoError(b, err)

		upToDate, err := nativeMTimeUpToDate(sourceRoot, outputInfo.ModTime())
		require.NoError(b, err)
		require.True(b, upToDate)
	}
	b.ReportMetric(float64(fileCount), "source_files/op")
	b.ReportMetric(float64(sourceBytes)/(1024*1024), "source_MiB/op")
}

func nativeMTimeUpToDate(sourceRoot string, outputTime time.Time) (bool, error) {
	upToDate := true
	err := filepath.WalkDir(sourceRoot, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || filepath.Ext(path) != ".yaml" {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		if info.ModTime().After(outputTime) {
			upToDate = false
			return fs.SkipAll
		}
		return nil
	})
	return upToDate, err
}

func createIssue2853Fixture(tb testing.TB, dir string, fileCount int, fileSize int64) {
	tb.Helper()

	taskfile := `version: '3'

tasks:

  timestamp-yaml:
    transform:
      matches:
		- path/to/folder/**/*.yaml
	  yields:
		- out/timestamp.txt
    cmds:
      - printf ok > out/timestamp.txt

  uncached-yaml:
    cmds:
      - printf ok > out/uncached.txt
`
	require.NoError(tb, os.WriteFile(filepath.Join(dir, "Taskfile.yml"), []byte(taskfile), 0o644))
	require.NoError(tb, os.MkdirAll(filepath.Join(dir, "out"), 0o755))

	for i := 1; i <= fileCount; i++ {
		subdir := filepath.Join(dir, "path", "to", "folder", fmt.Sprintf("%04d", i/100))
		require.NoError(tb, os.MkdirAll(subdir, 0o755))
		name := filepath.Join(subdir, fmt.Sprintf("file-%05d.yaml", i))
		createSparseFile(tb, name, fileSize)
	}
}

func createSparseFile(tb testing.TB, name string, size int64) {
	tb.Helper()

	file, err := os.OpenFile(name, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
	require.NoError(tb, err)
	defer func() {
		require.NoError(tb, file.Close())
	}()
	require.NoError(tb, file.Truncate(size))
}
