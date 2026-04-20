package balancer

import (
	"errors"
	"io"
	"time"
)

var ErrStalled = errors.New("stream stalled: no data received within timeout")

// IdleTimeoutReader wraps an io.ReadCloser and returns ErrStalled if no data is read within timeout.
type IdleTimeoutReader struct {
	inner   io.ReadCloser
	timeout time.Duration
	timer   *time.Timer
}

func NewIdleTimeoutReader(inner io.ReadCloser, timeout time.Duration) *IdleTimeoutReader {
	return &IdleTimeoutReader{
		inner:   inner,
		timeout: timeout,
		timer:   time.NewTimer(timeout),
	}
}

func (r *IdleTimeoutReader) Read(p []byte) (n int, err error) {
	// Reset timer on every read attempt? 
	// Or only if we actually get data?
	// If we get data, we definitely want to reset it.
	
	// We use a channel to wait for the read to complete or the timer to fire.
	type result struct {
		n   int
		err error
	}
	ch := make(chan result, 1)
	
	go func() {
		n, err := r.inner.Read(p)
		ch <- result{n, err}
	}()

	select {
	case res := <-ch:
		if !r.timer.Stop() {
			select {
			case <-r.timer.C:
			default:
			}
		}
		if res.n > 0 {
			r.timer.Reset(r.timeout)
		}
		return res.n, res.err
	case <-r.timer.C:
		r.inner.Close()
		<-ch // Wait for goroutine to exit to prevent data race
		return 0, ErrStalled
	}
}

func (r *IdleTimeoutReader) Close() error {
	r.timer.Stop()
	return r.inner.Close()
}
