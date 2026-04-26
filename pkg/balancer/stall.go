package balancer

import (
	"errors"
	"io"
	"sync"
	"time"
)

var ErrStalled = errors.New("stream stalled")

type IdleTimeoutReader struct {
	inner   io.ReadCloser
	timeout time.Duration
	timer   *time.Timer
	mu      sync.Mutex
}

func NewIdleTimeoutReader(inner io.ReadCloser, timeout time.Duration) *IdleTimeoutReader {
	r := &IdleTimeoutReader{
		inner:   inner,
		timeout: timeout,
	}
	r.timer = time.AfterFunc(timeout, func() {
		r.mu.Lock()
		defer r.mu.Unlock()
		r.inner.Close()
	})
	return r
}

func (r *IdleTimeoutReader) Read(p []byte) (int, error) {
	r.mu.Lock()
	r.timer.Reset(r.timeout)
	r.mu.Unlock()

	n, err := r.inner.Read(p)
	if err != nil && (errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF)) {
		r.timer.Stop()
		return n, err
	}

	if err != nil && errors.Is(err, io.ErrClosedPipe) {
		return n, ErrStalled
	}

	return n, err
}

func (r *IdleTimeoutReader) Close() error {
	r.mu.Lock()
	r.timer.Stop()
	r.mu.Unlock()
	return r.inner.Close()
}
