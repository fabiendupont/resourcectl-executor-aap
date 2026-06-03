// Copyright 2025 The infractl Authors
// SPDX-License-Identifier: Apache-2.0

package aap

import (
	"context"
	"errors"
	"sync/atomic"
)

// ErrMaxInFlight is returned when the maximum number of concurrent
// jobs has been reached.
var ErrMaxInFlight = errors.New("maximum in-flight jobs reached")

// RateLimiter tracks in-flight jobs and enforces a concurrency cap.
type RateLimiter struct {
	max     int64
	current atomic.Int64
}

// NewRateLimiter creates a limiter with the given max concurrent jobs.
// A max of 0 means unlimited.
func NewRateLimiter(max int) *RateLimiter {
	return &RateLimiter{max: int64(max)}
}

// Acquire increments the in-flight count. Returns ErrMaxInFlight if
// the limit is reached.
func (l *RateLimiter) Acquire(_ context.Context) error {
	if l.max <= 0 {
		l.current.Add(1)
		return nil
	}
	for {
		cur := l.current.Load()
		if cur >= l.max {
			return ErrMaxInFlight
		}
		if l.current.CompareAndSwap(cur, cur+1) {
			return nil
		}
	}
}

// Release decrements the in-flight count.
func (l *RateLimiter) Release() {
	l.current.Add(-1)
}

// InFlight returns the current number of in-flight jobs.
func (l *RateLimiter) InFlight() int64 {
	return l.current.Load()
}
