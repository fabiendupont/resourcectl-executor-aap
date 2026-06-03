// Copyright 2025 The infractl Authors
// SPDX-License-Identifier: Apache-2.0

package aap

import (
	"github.com/prometheus/client_golang/prometheus"
)

var (
	submitDuration = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Namespace: "infractl",
			Subsystem: "aap_executor",
			Name:      "submit_duration_seconds",
			Help:      "Duration of AAP job submission in seconds.",
			Buckets:   prometheus.DefBuckets,
		},
		[]string{"template", "status"},
	)

	pollDuration = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Namespace: "infractl",
			Subsystem: "aap_executor",
			Name:      "poll_duration_seconds",
			Help:      "Duration of AAP job status poll in seconds.",
			Buckets:   prometheus.DefBuckets,
		},
		[]string{"status"},
	)

	jobsInFlight = prometheus.NewGauge(
		prometheus.GaugeOpts{
			Namespace: "infractl",
			Subsystem: "aap_executor",
			Name:      "jobs_in_flight",
			Help:      "Number of AAP jobs currently in flight.",
		},
	)

	submitTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: "infractl",
			Subsystem: "aap_executor",
			Name:      "submit_total",
			Help:      "Total number of AAP job submissions.",
		},
		[]string{"template", "result"},
	)

	circuitBreakerState = prometheus.NewGauge(
		prometheus.GaugeOpts{
			Namespace: "infractl",
			Subsystem: "aap_executor",
			Name:      "circuit_state",
			Help:      "Circuit breaker state: 0=closed, 1=open, 2=half-open.",
		},
	)
)

func init() {
	prometheus.MustRegister(submitDuration, pollDuration, jobsInFlight, submitTotal, circuitBreakerState)
}
