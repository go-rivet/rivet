package output

import (
	"io"

	"github.com/go-rivet/rivet/internal/templater"
)

type Passthrough struct{}

func (Passthrough) WrapWriter(stdOut, stdErr io.Writer, _ string, _ *templater.Cache) (io.Writer, io.Writer, CloseFunc) {
	return stdOut, stdErr, func(error) error { return nil }
}
