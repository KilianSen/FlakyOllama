package metrics

import (
	"sync"
)

// EMA calculates the exponential moving average.
type EMA struct {
	value       float64
	alpha       float64
	initialized bool
	mu          sync.RWMutex
}

func NewEMA(alpha float64) *EMA {
	return &EMA{alpha: alpha}
}

func (e *EMA) Update(newValue float64) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if !e.initialized {
		e.value = newValue
		e.initialized = true
	} else {
		e.value = e.alpha*newValue + (1-e.alpha)*e.value
	}
}

func (e *EMA) Value() float64 {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.value
}
