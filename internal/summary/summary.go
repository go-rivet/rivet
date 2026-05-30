package summary

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/go-rivet/rivet/pkg/rivet/taskfile/ast"
	"github.com/go-rivet/rivet/pkg/rlog"
)

func PrintTasks(ctx context.Context, t *ast.Taskfile, c []string) {
	for i, call := range c {
		PrintSpaceBetweenSummaries(ctx, i)
		if task, ok := t.Tasks.Get(call); ok {
			PrintTask(ctx, task)
		}
	}
}

func PrintSpaceBetweenSummaries(ctx context.Context, i int) {
	spaceRequired := i > 0
	if !spaceRequired {
		return
	}

	rlog.OutRawf(ctx, rlog.Default, "\n")
	rlog.OutRawf(ctx, rlog.Default, "\n")
}

func PrintTask(ctx context.Context, t *ast.Task) {
	printTaskName(ctx, t)
	printTaskDescribingText(ctx, t)
	printTaskVars(ctx, t)
	printTaskRequires(ctx, t)
	printTaskDependencies(ctx, t)
	printTaskAliases(ctx, t)
	printTaskCommands(ctx, t)
}

func printTaskDescribingText(ctx context.Context, t *ast.Task) {
	if hasSummary(t) {
		printTaskSummary(ctx, t)
	} else if hasDescription(t) {
		printTaskDescription(ctx, t)
	} else {
		printNoDescriptionOrSummary(ctx)
	}
}

func hasSummary(t *ast.Task) bool {
	return t.Summary != ""
}

func printTaskSummary(ctx context.Context, t *ast.Task) {
	lines := strings.Split(t.Summary, "\n")
	for i, line := range lines {
		notLastLine := i+1 < len(lines)
		if notLastLine || line != "" {
			rlog.OutRawf(ctx, rlog.Default, "%s\n", line)
		}
	}
}

func printTaskName(ctx context.Context, t *ast.Task) {
	rlog.OutRawf(ctx, rlog.Default, "task: ")
	rlog.OutRawf(ctx, rlog.Green, "%s\n", t.Name())
	rlog.OutRawf(ctx, rlog.Default, "\n")
}

func printTaskAliases(ctx context.Context, t *ast.Task) {
	if len(t.Aliases) == 0 {
		return
	}
	rlog.OutRawf(ctx, rlog.Default, "\n")
	rlog.OutRawf(ctx, rlog.Default, "aliases:\n")
	for _, alias := range t.Aliases {
		rlog.OutRawf(ctx, rlog.Default, " - ")
		rlog.OutRawf(ctx, rlog.Cyan, "%s\n", alias)
	}
}

func hasDescription(t *ast.Task) bool {
	return t.Desc != ""
}

func printTaskDescription(ctx context.Context, t *ast.Task) {
	rlog.OutRawf(ctx, rlog.Default, "%s\n", t.Desc)
}

func printNoDescriptionOrSummary(ctx context.Context) {
	rlog.OutRawf(ctx, rlog.Default, "(task does not have description or summary)\n")
}

func printTaskDependencies(ctx context.Context, t *ast.Task) {
	if len(t.Deps) == 0 {
		return
	}

	rlog.OutRawf(ctx, rlog.Default, "\n")
	rlog.OutRawf(ctx, rlog.Default, "dependencies:\n")

	for _, d := range t.Deps {
		rlog.OutRawf(ctx, rlog.Default, " - %s\n", d.Task)
	}
}

func printTaskCommands(ctx context.Context, t *ast.Task) {
	if len(t.Cmds) == 0 {
		return
	}

	rlog.OutRawf(ctx, rlog.Default, "\n")
	rlog.OutRawf(ctx, rlog.Default, "commands:\n")
	for _, c := range t.Cmds {
		isCommand := c.Cmd != ""
		rlog.OutRawf(ctx, rlog.Default, " - ")
		if isCommand {
			rlog.OutRawf(ctx, rlog.Yellow, "%s\n", c.Cmd)
		} else {
			rlog.OutRawf(ctx, rlog.Green, "Task: %s\n", c.Task)
		}
	}
}

func printTaskVars(ctx context.Context, t *ast.Task) {
	if t.Vars == nil || t.Vars.Len() == 0 {
		return
	}

	osEnvVars := getEnvVarNames()

	hasNonEnvVars := false
	for key := range t.Vars.All() {
		if !isEnvVar(key, osEnvVars) {
			hasNonEnvVars = true
			break
		}
	}

	if !hasNonEnvVars {
		return
	}

	rlog.OutRawf(ctx, rlog.Default, "\n")
	rlog.OutRawf(ctx, rlog.Default, "vars:\n")

	for key, value := range t.Vars.All() {
		// Only display variables that are not from OS environment or Taskfile env
		if !isEnvVar(key, osEnvVars) {
			formattedValue := formatVarValue(value)
			rlog.OutRawf(ctx, rlog.Yellow, "  %s: %s\n", key, formattedValue)
		}
	}
}

// formatVarValue formats a variable value based on its type.
// Handles static values, shell commands (sh:), references (ref:), and maps.
func formatVarValue(v ast.Var) string {
	// Shell command - check this first before Value
	// because dynamic vars may have both Sh and an empty Value
	if v.Sh != nil {
		return fmt.Sprintf("sh: %s", *v.Sh)
	}

	// Reference
	if v.Ref != "" {
		return fmt.Sprintf("ref: %s", v.Ref)
	}

	// Static value
	if v.Value != nil {
		// Check if it's a map or complex type
		if m, ok := v.Value.(map[string]any); ok {
			return formatMap(m, 4)
		}
		// Simple string value
		return fmt.Sprintf(`"%v"`, v.Value)
	}

	return `""`
}

// formatMap formats a map value with proper indentation for YAML.
func formatMap(m map[string]any, indent int) string {
	if len(m) == 0 {
		return "{}"
	}

	var result strings.Builder
	result.WriteString("\n")
	spaces := strings.Repeat(" ", indent)

	for k, v := range m {
		result.WriteString(fmt.Sprintf("%s%s: %v\n", spaces, k, v)) //nolint:staticcheck
	}

	return result.String()
}

func printTaskRequires(ctx context.Context, t *ast.Task) {
	if t.Requires == nil || len(t.Requires.Vars) == 0 {
		return
	}

	rlog.OutRawf(ctx, rlog.Default, "\n")
	rlog.OutRawf(ctx, rlog.Default, "requires:\n")
	rlog.OutRawf(ctx, rlog.Default, "  vars:\n")

	for _, v := range t.Requires.Vars {
		if v.Enum != nil && len(v.Enum.Value) > 0 {
			rlog.OutRawf(ctx, rlog.Yellow, "    - %s:\n", v.Name)
			rlog.OutRawf(ctx, rlog.Yellow, "        enum:\n")
			for _, enumValue := range v.Enum.Value {
				rlog.OutRawf(ctx, rlog.Yellow, "          - %s\n", enumValue)
			}
		} else if v.Enum != nil && v.Enum.Ref != "" {
			rlog.OutRawf(ctx, rlog.Yellow, "    - %s:\n", v.Name)
			rlog.OutRawf(ctx, rlog.Yellow, "        enum:\n")
			rlog.OutRawf(ctx, rlog.Yellow, "          ref: %s\n", v.Enum.Ref)
		} else {
			rlog.OutRawf(ctx, rlog.Yellow, "    - %s\n", v.Name)
		}
	}
}

func getEnvVarNames() map[string]bool {
	envMap := make(map[string]bool)
	for _, e := range os.Environ() {
		parts := strings.SplitN(e, "=", 2)
		if len(parts) > 0 {
			envMap[parts[0]] = true
		}
	}
	return envMap
}

// isEnvVar checks if a variable is from OS environment or auto-generated by Task.
func isEnvVar(key string, envVars map[string]bool) bool {
	// Filter out auto-generated Task variables
	if strings.HasPrefix(key, "TASK_") ||
		strings.HasPrefix(key, "CLI_") ||
		strings.HasPrefix(key, "ROOT_") ||
		key == "TASK" ||
		key == "TASKFILE" ||
		key == "TASKFILE_DIR" ||
		key == "USER_WORKING_DIR" ||
		key == "ALIAS" ||
		key == "MATCH" ||
		key == "PATH_LIST_SEPARATOR" ||
		key == "FILE_PATH_SEPARATOR" {
		return true
	}
	return envVars[key]
}
