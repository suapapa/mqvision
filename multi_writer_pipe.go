package main

import (
	"io"
	"sync"
)

type SingleInMultiOutPipeReader struct {
	io.Reader
	closer io.Closer
}

func (r *SingleInMultiOutPipeReader) Close() error {
	if r.closer != nil {
		return r.closer.Close()
	}
	return nil
}

type SingleInMultiOutPipeWriter struct {
	writers []io.Writer
	closers []io.Closer
	mu      sync.Mutex
	closed  bool
}

func (w *SingleInMultiOutPipeWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()

	if w.closed {
		return 0, io.ErrClosedPipe
	}

	// Write to all writers
	// If any writer fails, we still write to others but return the error
	var firstErr error
	for _, writer := range w.writers {
		n, err := writer.Write(p)
		if err != nil && firstErr == nil {
			firstErr = err
		}
		if n != len(p) && firstErr == nil {
			firstErr = io.ErrShortWrite
		}
	}

	if firstErr != nil {
		return len(p), firstErr
	}
	return len(p), nil
}

func (w *SingleInMultiOutPipeWriter) Close() error {
	w.mu.Lock()
	defer w.mu.Unlock()

	if w.closed {
		return nil
	}
	w.closed = true

	var errs []error
	for _, closer := range w.closers {
		if err := closer.Close(); err != nil {
			errs = append(errs, err)
		}
	}

	if len(errs) > 0 {
		return errs[0] // Return first error
	}
	return nil
}

// SingleInMultiOutPipe creates a single Writer and multiple Readers.
// Writing to the Writer will broadcast the data to all Readers.
// readerCount specifies how many Readers to create.
func SingleInMultiOutPipe(readerCount int) (*SingleInMultiOutPipeWriter, []*SingleInMultiOutPipeReader) {
	writers := make([]io.Writer, readerCount)
	closers := make([]io.Closer, readerCount)
	readers := make([]*SingleInMultiOutPipeReader, readerCount)

	for i := 0; i < readerCount; i++ {
		pr, pw := io.Pipe()
		writers[i] = pw
		closers[i] = pw
		readers[i] = &SingleInMultiOutPipeReader{
			Reader: pr,
			closer: pr,
		}
	}

	writer := &SingleInMultiOutPipeWriter{
		writers: writers,
		closers: closers,
		closed:  false,
	}

	return writer, readers
}
