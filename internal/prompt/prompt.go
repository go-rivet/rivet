package prompt

import (
	"bufio"
	"context"
	"errors"
	"os"
	"slices"
	"strings"

	"github.com/go-rivet/rivet/internal/term"
	"github.com/go-rivet/rivet/pkg/rlog"
)

var (
	ErrPromptCancelled = errors.New("prompt cancelled")
	ErrNoTerminal      = errors.New("no terminal")
)

func Prompt(ctx context.Context, prompt string, defaultValue string, assumeTerm bool, assumeYes bool, continueValues ...string) error {
	if assumeYes {
		rlog.Outf(ctx, rlog.Default, "%s [assuming yes]\n", prompt)
		return nil
	}

	if !assumeTerm && !term.IsTerminal() {
		return ErrNoTerminal
	}

	if len(continueValues) == 0 {
		return errors.New("no continue values provided")
	}

	rlog.Outf(ctx, rlog.Default, "%s [%s/%s]: ", prompt, strings.ToLower(continueValues[0]), strings.ToUpper(defaultValue))

	reader := bufio.NewReader(os.Stdin)
	input, err := reader.ReadString('\n')
	if err != nil {
		return err
	}

	input = strings.TrimSpace(strings.ToLower(input))
	if !slices.Contains(continueValues, input) {
		return ErrPromptCancelled
	}

	return nil
}
