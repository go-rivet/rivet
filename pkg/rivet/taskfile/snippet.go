package taskfile

import (
	"fmt"
	"strings"
)

// GenerateSnippet extracts a text window around a specific line for debugging context.
func GenerateSnippet(content []byte, targetLine, padding int) string {
	lines := strings.Split(string(content), "\n")
	totalLines := len(lines)
	if totalLines == 0 || targetLine < 1 {
		return ""
	}

	// Calculate a safe 1-indexed window boundary
	start := targetLine - padding
	if start < 1 {
		start = 1
	}
	end := targetLine + padding
	if end > totalLines {
		end = totalLines
	}

	var builder strings.Builder
	for i := start; i <= end; i++ {
		prefix := "  "
		if i == targetLine {
			prefix = "> " // Visual indicator matching your error target line
		}
		// Formats lines with aligned line numbers
		fmt.Fprintf(&builder, "%s%3d | %s\n", prefix, i, lines[i-1])
	}
	return builder.String()
}
