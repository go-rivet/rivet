package output

import (
	"bytes"
	"io"
	"sync"

	"github.com/go-rivet/rivet/internal/templater"
)

var interleavedMutex sync.Mutex

type Interleaved struct {
	mutex sync.Mutex
}

func (Interleaved) WrapWriter(stdOut, stdErr io.Writer, _ string, _ *templater.Cache) (io.Writer, io.Writer, CloseFunc) {
	outW := &lineWriter{writer: stdOut}
	errW := &lineWriter{writer: stdErr}
	return outW, errW, func(error) error {
		if err := outW.flush(); err != nil {
			_ = errW.flush()
			return err
		}
		return errW.flush()
	}
}

type lineWriter struct {
	writer io.Writer
	buff   bytes.Buffer
	mu     sync.Mutex
}

func (lw *lineWriter) Write(p []byte) (int, error) {
	lw.mu.Lock()
	defer lw.mu.Unlock()

	lw.buff.Write(p)
	idx := bytes.LastIndexByte(lw.buff.Bytes(), '\n')
	if idx < 0 {
		return len(p), nil
	}
	return len(p), lw.emit(lw.buff.Next(idx + 1))
}

func (lw *lineWriter) flush() error {
	lw.mu.Lock()
	defer lw.mu.Unlock()

	if lw.buff.Len() == 0 {
		return nil
	}
	return lw.emit(lw.buff.Next(lw.buff.Len()))
}

func (lw *lineWriter) emit(b []byte) error {
	interleavedMutex.Lock()
	defer interleavedMutex.Unlock()

	_, err := lw.writer.Write(b)
	return err
}
