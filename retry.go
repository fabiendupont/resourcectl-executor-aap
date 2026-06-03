// Copyright 2025 The infractl Authors
// SPDX-License-Identifier: Apache-2.0

package aap

import (
	"math"
	"net/http"
	"time"
)

// RetryConfig controls retry behavior for transient HTTP failures.
type RetryConfig struct {
	MaxAttempts int
	BaseDelay   time.Duration
	MaxDelay    time.Duration
}

// DefaultRetryConfig returns sensible retry defaults.
func DefaultRetryConfig() RetryConfig {
	return RetryConfig{
		MaxAttempts: 3,
		BaseDelay:   500 * time.Millisecond,
		MaxDelay:    10 * time.Second,
	}
}

// retryTransport wraps an http.RoundTripper with retry logic for
// transient failures (5xx, connection errors).
type retryTransport struct {
	base   http.RoundTripper
	config RetryConfig
}

func (t *retryTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	var resp *http.Response
	var err error

	for attempt := 0; attempt <= t.config.MaxAttempts; attempt++ {
		if attempt > 0 {
			delay := t.backoff(attempt)
			select {
			case <-req.Context().Done():
				return nil, req.Context().Err()
			case <-time.After(delay):
			}
		}

		resp, err = t.base.RoundTrip(req)
		if err != nil {
			continue
		}

		if !isRetryable(resp.StatusCode) {
			return resp, nil
		}

		if attempt < t.config.MaxAttempts {
			resp.Body.Close()
		}
	}

	return resp, err
}

func (t *retryTransport) backoff(attempt int) time.Duration {
	delay := time.Duration(float64(t.config.BaseDelay) * math.Pow(2, float64(attempt-1)))
	if delay > t.config.MaxDelay {
		delay = t.config.MaxDelay
	}
	return delay
}

func isRetryable(statusCode int) bool {
	return statusCode == http.StatusTooManyRequests ||
		statusCode == http.StatusServiceUnavailable ||
		statusCode == http.StatusGatewayTimeout ||
		statusCode == http.StatusBadGateway
}

// WithRetry wraps a transport with retry logic.
func WithRetry(base http.RoundTripper, cfg RetryConfig) http.RoundTripper {
	return &retryTransport{base: base, config: cfg}
}
