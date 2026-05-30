package rivet

import (
	"log/slog"
	"os"
	"testing"

	//"github.com/go-rivet/rivet/internal/flags"// fixmet circular import, problem with flags I think.
	"github.com/go-rivet/rivet/pkg/rlog"
)

// Define a package-level, thread-safe variable to hold the level
var testLogLevel = &slog.LevelVar{}

// var globalRouter *TestLogRouter

func TestMain(m *testing.M) {
	rlog.SetTestMode()

	// Default to Info level for all tests, run() will modify.
	testLogLevel.Set(slog.LevelInfo)
	fallbackHandler := rlog.NewCliHandler(os.Stdout, os.Stderr, false, &slog.HandlerOptions{Level: testLogLevel})

	// Register the context router globally
	slog.SetDefault(slog.New(rlog.NewTestLogRouter(fallbackHandler)))

	os.Exit(m.Run())
}
