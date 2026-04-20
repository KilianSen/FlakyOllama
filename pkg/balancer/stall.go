package balancer

import (
	"errors"
	"io"
	"sync"
	"time"
)

var ErrStalled = errors.New("stream stalled: no data received within timeout")

// IdleTimeoutReader wraps an io.ReadCloser and returns ErrStalled if no data is read within timeout.
type IdleTimeoutReader struct {
	inner   io.ReadCloser
	timeout time.Duration
	timer   *time.Timer

	dataCh   chan readResult
	stopCh   chan struct{}
	doneCh   chan struct{}
	once     sync.Once
	leftover []byte
}

type readResult struct {
	p   []byte
	err error
}

func NewIdleTimeoutReader(inner io.ReadCloser, timeout time.Duration) *IdleTimeoutReader {
	return &IdleTimeoutReader{
		inner:   inner,
		timeout: timeout,
		timer:   time.NewTimer(timeout),
		dataCh:  make(chan readResult),
		stopCh:  make(chan struct{}),
		doneCh:  make(chan struct{}),
	}
}

func (r *IdleTimeoutReader) Read(p []byte) (n int, err error) {
	// 1. Check if we have leftover data from a previous background read
	if len(r.leftover) > 0 {
		n = copy(p, r.leftover)
		r.leftover = r.leftover[n:]
		return n, nil
	}

	// 2. Start background reader once
	r.once.Do(func() {
		go r.backgroundRead()
	})

	// 3. Reset timer
	if !r.timer.Stop() {
		select {
		case <-r.timer.C:
		default:
		}
	}
	r.timer.Reset(r.timeout)

	// 4. Wait for data or timeout
	select {
	case res, ok := <-r.dataCh:
		if !ok {
			return 0, io.EOF
		}
		if res.err != nil {
			return 0, res.err
		}

		n = copy(p, res.p)
		if n < len(res.p) {
			// Store leftovers
			r.leftover = res.p[n:]
		}
		return n, nil
	case <-r.timer.C:
		r.inner.Close()
		<-r.doneCh // Wait for goroutine to exit to prevent data race
		return 0, ErrStalled
	case <-r.stopCh:
		return 0, io.EOF
	}
}

func (r *IdleTimeoutReader) backgroundRead() {
	defer close(r.doneCh)
	defer close(r.dataCh)
	for {
		buf := make([]byte, 32*1024)
		n, err := r.inner.Read(buf)
		if n > 0 {
			// We must send a copy or ensure the slice is not reused immediately.
			// Since we allocate a new buf in each iteration, it's safe.
			select {
			case r.dataCh <- readResult{p: buf[:n]}:
			case <-r.stopCh:
				return
			}
		}
		if err != nil {
			select {
			case r.dataCh <- readResult{err: err}:
			case <-r.stopCh:
			}
			return
		}
	}
}

func (r *IdleTimeoutReader) Close() error {
	r.timer.Stop()
	r.once.Do(func() {})
	// Signal stop before closing inner to avoid potential races
	select {
	case <-r.stopCh:
		// already closed
	default:
		close(r.stopCh)
	}
	return r.inner.Close()
}
