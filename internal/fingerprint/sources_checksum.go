package fingerprint

import (
	"crypto/sha256"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/go-rivet/rivet/internal/filepathext"
	"github.com/go-rivet/rivet/pkg/rivet/taskfile/ast"
)

// ChecksumChecker validates if a task is up to date by calculating its source
// files checksum
type ChecksumChecker struct {
	tempDir string
	dry     bool
}

func NewChecksumChecker(tempDir string, dry bool) *ChecksumChecker {
	return &ChecksumChecker{
		tempDir: tempDir,
		dry:     dry,
	}
}

func (checker *ChecksumChecker) IsUpToDate(t *ast.Task) (bool, error) {
	if len(t.Sources) == 0 {
		return false, nil
	}

	checksumFile := checker.checksumFilePath(t)

	data, _ := os.ReadFile(checksumFile)
	oldHash := strings.TrimSpace(string(data))

	newHash, err := checker.checksum(t)
	if err != nil {
		return false, nil
	}

	if !checker.dry && oldHash != newHash {
		_ = os.MkdirAll(filepathext.SmartJoin(checker.tempDir, "checksum"), 0o755)
		if err = os.WriteFile(checksumFile, []byte(newHash+"\n"), 0o644); err != nil {
			return false, err
		}
	}

	if len(t.Generates) > 0 {
		// For each specified 'generates' field, check whether the files actually exist
		for _, g := range t.Generates {
			// Exclusion patterns don't represent output files; skip them.
			if g.Negate {
				continue
			}
			generates, err := glob(t.Dir, g.Glob)
			if os.IsNotExist(err) {
				return false, nil
			}
			if err != nil {
				return false, err
			}
			if len(generates) == 0 {
				return false, nil
			}
		}
	}

	return oldHash == newHash, nil
}

func (checker *ChecksumChecker) Value(t *ast.Task) (any, error) {
	return checker.checksum(t)
}

func (checker *ChecksumChecker) OnError(t *ast.Task) error {
	if len(t.Sources) == 0 {
		return nil
	}
	return os.Remove(checker.checksumFilePath(t))
}

func (*ChecksumChecker) Kind() string {
	return "checksum"
}

func (c *ChecksumChecker) checksum(t *ast.Task) (string, error) {
	sources, err := Globs(t.Dir, t.Sources)
	if err != nil {
		return "", err
	}

	h := sha256.New()
	buf := make([]byte, 128*1024)

	for _, f := range sources {
		// Also sum the filename, so checksum changes when renaming a file
		if _, err := io.CopyBuffer(h, strings.NewReader(filepath.Base(f)), buf); err != nil {
			return "", err
		}
		file, err := os.Open(f)
		if err != nil {
			return "", err
		}
		if _, err = io.CopyBuffer(h, file, buf); err != nil {
			_ = file.Close() // Ensure descriptor is freed if copying fails
			return "", err
		}
		_ = file.Close()
	}

	return fmt.Sprintf("%x", h.Sum(nil)), nil
}

func (checker *ChecksumChecker) checksumFilePath(t *ast.Task) string {
	return filepath.Join(checker.tempDir, "checksum", normalizeFilename(t.Name()))
}

var checksumFilenameRegexp = regexp.MustCompile("[^A-z0-9]")

// replaces invalid characters on filenames with "-"
func normalizeFilename(f string) string {
	return checksumFilenameRegexp.ReplaceAllString(f, "-")
}
