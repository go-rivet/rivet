package args

import (
	"strings"

	task "github.com/go-rivet/rivet/pkg/rivet"
	"github.com/go-rivet/rivet/pkg/rivet/taskfile/ast"
	"mvdan.cc/sh/v3/syntax"
)

// Parse parses command line argument: tasks and global variables
func Parse(args ...string) ([]*task.Call, *ast.Vars) {
	calls := []*task.Call{}
	globals := ast.NewVars()

	for _, arg := range args {
		if !strings.Contains(arg, "=") {
			calls = append(calls, &task.Call{Task: arg})
			continue
		}

		name, value := splitVar(arg)
		globals.Set(name, ast.Var{Value: value})
	}

	return calls, globals
}

func ToQuotedString(args []string) (string, error) {
	var quotedCliArgs []string
	for _, arg := range args {
		quotedCliArg, err := syntax.Quote(arg, syntax.LangBash)
		if err != nil {
			return "", err
		}
		quotedCliArgs = append(quotedCliArgs, quotedCliArg)
	}
	return strings.Join(quotedCliArgs, " "), nil
}

func splitVar(s string) (string, string) {
	pair := strings.SplitN(s, "=", 2)
	return pair[0], pair[1]
}
