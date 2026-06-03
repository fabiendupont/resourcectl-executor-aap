// Copyright 2025 The infractl Authors
// SPDX-License-Identifier: Apache-2.0

package aap

import (
	"errors"
	"net/http"
	"sync"
	"time"
)

// ErrCircuitOpen is returned when the circuit breaker is open and
// requests are being rejected without reaching AAP.
var ErrCircuitOpen = errors.New("circuit breaker is open: AAP Controller unreachable")

// CircuitConfig controls circuit breaker behavior.
type CircuitConfig struct {
	FailureThreshold int
	ResetTimeout     time.Duration
}

// DefaultCircuitConfig returns sensible circuit breaker defaults.
func DefaultCircuitConfig() CircuitConfig {
	return CircuitConfig{
		FailureThreshold: 5,
		ResetTimeout:     30 * time.Second,
	}
}

type circuitState int

const (
	circuitClosed circuitState = iota
	circuitOpen
	circuitHalfOpen
)

// circuitTransport wraps an http.RoundTripper with circuit breaker logic.
type circuitTransport struct {
	base   http.RoundTripper
	config CircuitConfig

	mu           sync.Mutex
	state        circuitState
	failures     int
	lastFailure  time.Time
}

func (t *circuitTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	t.mu.Lock()
	switch t.state {
	case circuitOpen:
		if time.Since(t.lastFailure) > t.config.ResetTimeout {
			t.state = circuitHalfOpen
			t.mu.Unlock()
		} else {
			t.mu.Unlock()
			return nil, ErrCircuitOpen
		}
	default:
		t.mu.Unlock()
	}

	resp, err := t.base.RoundTrip(req)

	t.mu.Lock()
	defer t.mu.Unlock()

	if err != nil || (resp != nil && resp.StatusCode >= 500) {
		t.failures++
		t.lastFailure = time.Now()
		if t.failures >= t.config.FailureThreshold {
			t.state = circuitOpen
		}
		return resp, err
	}

	t.failures = 0
	t.state = circuitClosed
	return resp, nil
}

// WithCircuitBreaker wraps a transport with circuit breaker logic.
func WithCircuitBreaker(base http.RoundTripper, cfg CircuitConfig) http.RoundTripper {
	return &circuitTransport{base: base, config: cfg}
}
