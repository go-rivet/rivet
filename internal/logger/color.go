package logger

import (
	"fmt"
	"io"
	"os"
	"slices"
	"strconv"
	"strings"

	"github.com/go-rivet/rivet/internal/term"
)

var (
	// State variable to toggle coloring globally
	NoColor = false

	attrsReset       = envColor("COLOR_RESET", 0)
	attrsFgBlue      = envColor("COLOR_BLUE", 34)
	attrsFgGreen     = envColor("COLOR_GREEN", 32)
	attrsFgCyan      = envColor("COLOR_CYAN", 36)
	attrsFgYellow    = envColor("COLOR_YELLOW", 33)
	attrsFgMagenta   = envColor("COLOR_MAGENTA", 35)
	attrsFgRed       = envColor("COLOR_RED", 31)
	attrsFgHiBlue    = envColor("COLOR_BRIGHT_BLUE", 94)
	attrsFgHiGreen   = envColor("COLOR_BRIGHT_GREEN", 92)
	attrsFgHiCyan    = envColor("COLOR_BRIGHT_CYAN", 96)
	attrsFgHiYellow  = envColor("COLOR_BRIGHT_YELLOW", 93)
	attrsFgHiMagenta = envColor("COLOR_BRIGHT_MAGENTA", 95)
	attrsFgHiRed     = envColor("COLOR_BRIGHT_RED", 91)
)

func init() {
	// 1. Enforce standard NO_COLOR specification overrides
	if os.Getenv("NO_COLOR") != "" || os.Getenv("TERM") == "dumb" {
		NoColor = true
		return
	}

	// 2. Disable color formatting if writing out to an inverted stream (like file redirects)
	if !term.IsTerminal() {
		NoColor = true
		return
	}

	// 3. Initialize Windows Virtual Terminal Processing engine natively
	initTerminal()
}

type (
	Color     func() PrintFunc
	PrintFunc func(io.Writer, string, ...any)
)

func envColor(name string, defaultColor int) []int {
	// Native standard library lookup bypasses the internal/env package entirely!
	override := os.Getenv(name)
	if override == "" {
		return []int{defaultColor}
	}

	attributeStrs := strings.Split(override, ",")
	if len(attributeStrs) == 3 {
		attributeStrs = slices.Concat([]string{"38", "2"}, attributeStrs)
	} else {
		attributeStrs = strings.Split(override, ";")
	}

	attributes := make([]int, len(attributeStrs))
	for i, attributeStr := range attributeStrs {
		attribute, err := strconv.Atoi(attributeStr)
		if err != nil {
			return []int{defaultColor}
		}
		attributes[i] = attribute
	}

	return attributes
}

func newFprintfFunc(attrs []int) PrintFunc {
	var codes []string
	for _, attr := range attrs {
		codes = append(codes, strconv.Itoa(attr))
	}
	sequence := "\033[" + strings.Join(codes, ";") + "m"
	clearSeq := "\033[0m"

	return func(w io.Writer, format string, a ...any) {
		msg := fmt.Sprintf(format, a...)
		// Guard clause: skip escape strings if color mode is disabled
		if NoColor {
			_, _ = fmt.Fprint(w, msg)
			return
		}
		_, _ = fmt.Fprint(w, sequence+msg+clearSeq)
	}
}

func Default() PrintFunc {
	return newFprintfFunc(attrsReset)
}

func Blue() PrintFunc {
	return newFprintfFunc(attrsFgBlue)
}

func Green() PrintFunc {
	return newFprintfFunc(attrsFgGreen)
}

func Cyan() PrintFunc {
	return newFprintfFunc(attrsFgCyan)
}

func Yellow() PrintFunc {
	return newFprintfFunc(attrsFgYellow)
}

func Magenta() PrintFunc {
	return newFprintfFunc(attrsFgMagenta)
}

func Red() PrintFunc {
	return newFprintfFunc(attrsFgRed)
}

func BrightBlue() PrintFunc {
	return newFprintfFunc(attrsFgHiBlue)
}

func BrightGreen() PrintFunc {
	return newFprintfFunc(attrsFgHiGreen)
}

func BrightCyan() PrintFunc {
	return newFprintfFunc(attrsFgHiCyan)
}

func BrightYellow() PrintFunc {
	return newFprintfFunc(attrsFgHiYellow)
}

func BrightMagenta() PrintFunc {
	return newFprintfFunc(attrsFgHiMagenta)
}

func BrightRed() PrintFunc {
	return newFprintfFunc(attrsFgHiRed)
}

// newSprintFunc creates a helper that returns a colored string instead of writing to a stream
func newSprintFunc(attrs []int) func(string, ...any) string {
	var codes []string
	for _, attr := range attrs {
		codes = append(codes, strconv.Itoa(attr))
	}
	sequence := "\033[" + strings.Join(codes, ";") + "m"
	clearSeq := "\033[0m"

	return func(format string, a ...any) string {
		msg := fmt.Sprintf(format, a...)
		if NoColor {
			return msg
		}
		return sequence + msg + clearSeq
	}
}

// RedString replaces color.RedString natively
func RedString(format string, a ...any) string {
	return newSprintFunc(attrsFgRed)(format, a...)
}
