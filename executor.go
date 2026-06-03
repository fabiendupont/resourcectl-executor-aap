// Copyright 2025 The infractl Authors
// SPDX-License-Identifier: Apache-2.0

// Package aap implements a workflow.Executor that delegates to Ansible
// Automation Platform's Controller API. Handler.Ref is interpreted as
// the AAP Job Template name. Handler.Metadata can specify:
//   - "template_type": "workflow" to launch a Workflow Job Template
//     instead of a regular Job Template (default: "job")
package aap

import (
	"context"
	"fmt"
	"strconv"
	"sync"
	"time"

	"github.com/rs/zerolog"

	"github.com/fabiendupont/infractl/workflow"
)

// ExecutorConfig holds configuration for the AAP executor.
type ExecutorConfig struct {
	MaxInFlight int // 0 = unlimited
}

// Executor implements workflow.Executor by talking to AAP Controller.
type Executor struct {
	client  *Client
	limiter *RateLimiter
	logger  zerolog.Logger

	mu       sync.Mutex
	inflight map[string]struct{} // tracked run IDs for drain
	draining bool
}

// NewExecutor creates an AAP executor with the given client and config.
func NewExecutor(client *Client, logger zerolog.Logger, cfg ExecutorConfig) *Executor {
	return &Executor{
		client:   client,
		limiter:  NewRateLimiter(cfg.MaxInFlight),
		logger:   logger.With().Str("component", "aap-executor").Logger(),
		inflight: make(map[string]struct{}),
	}
}

// Healthy checks that the AAP Controller is reachable.
func (e *Executor) Healthy(ctx context.Context) error {
	return e.client.Healthy(ctx)
}

// Submit launches an AAP Job Template (or Workflow Job Template) and
// returns a Run with the AAP job ID.
func (e *Executor) Submit(ctx context.Context, handler workflow.Handler, input map[string]interface{}) (*workflow.Run, error) {
	e.mu.Lock()
	if e.draining {
		e.mu.Unlock()
		return nil, fmt.Errorf("executor is draining, not accepting new submissions")
	}
	e.mu.Unlock()

	if err := e.limiter.Acquire(ctx); err != nil {
		submitTotal.WithLabelValues(handler.Ref, "rate_limited").Inc()
		return nil, fmt.Errorf("rate limited: %w", err)
	}

	start := time.Now()

	req := LaunchRequest{ExtraVars: input}
	templateType := "job"
	if handler.Metadata != nil {
		if t, ok := handler.Metadata["template_type"]; ok {
			templateType = t
		}
	}

	var resp *LaunchResponse
	var err error

	switch templateType {
	case "workflow":
		resp, err = e.client.LaunchWorkflowTemplate(ctx, handler.Ref, req)
	default:
		resp, err = e.client.LaunchJobTemplate(ctx, handler.Ref, req)
	}

	duration := time.Since(start).Seconds()

	if err != nil {
		e.limiter.Release()
		submitDuration.WithLabelValues(handler.Ref, "error").Observe(duration)
		submitTotal.WithLabelValues(handler.Ref, "error").Inc()
		e.logger.Error().Err(err).Str("template", handler.Ref).Msg("submit failed")
		return nil, fmt.Errorf("launching AAP template %q: %w", handler.Ref, err)
	}

	runID := strconv.Itoa(resp.ID)

	e.mu.Lock()
	e.inflight[runID] = struct{}{}
	e.mu.Unlock()
	jobsInFlight.Set(float64(e.limiter.InFlight()))

	submitDuration.WithLabelValues(handler.Ref, "ok").Observe(duration)
	submitTotal.WithLabelValues(handler.Ref, "ok").Inc()
	e.logger.Info().Str("template", handler.Ref).Str("job_id", runID).Msg("job submitted")

	return &workflow.Run{
		ID:     runID,
		Status: workflow.RunPending,
	}, nil
}

// Poll checks the status of an AAP job and maps it to a workflow.Run.
func (e *Executor) Poll(ctx context.Context, runID string) (*workflow.Run, error) {
	start := time.Now()

	job, err := e.client.GetJob(ctx, runID)
	if err != nil {
		pollDuration.WithLabelValues("error").Observe(time.Since(start).Seconds())
		return nil, err
	}

	run := &workflow.Run{
		ID:     strconv.Itoa(job.ID),
		Status: mapAAPStatus(job.Status),
	}

	if job.ResultTraceback != "" {
		run.Error = job.ResultTraceback
	}

	pollDuration.WithLabelValues(string(run.Status)).Observe(time.Since(start).Seconds())

	if run.Status == workflow.RunCompleted || run.Status == workflow.RunFailed {
		e.releaseJob(runID)
		e.logger.Info().Str("job_id", runID).Str("status", string(run.Status)).Msg("job finished")
	}

	return run, nil
}

// Cancel cancels a running AAP job.
func (e *Executor) Cancel(ctx context.Context, runID string) error {
	err := e.client.CancelJob(ctx, runID)
	if err == nil {
		e.releaseJob(runID)
		e.logger.Info().Str("job_id", runID).Msg("job cancelled")
	}
	return err
}

// Drain stops accepting new submissions and waits for all in-flight
// jobs to complete (or the context to expire).
func (e *Executor) Drain(ctx context.Context) error {
	e.mu.Lock()
	e.draining = true
	e.mu.Unlock()

	e.logger.Info().Int64("in_flight", e.limiter.InFlight()).Msg("draining in-flight jobs")

	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	for {
		if e.limiter.InFlight() == 0 {
			e.logger.Info().Msg("all jobs drained")
			return nil
		}
		select {
		case <-ctx.Done():
			return fmt.Errorf("drain timed out with %d jobs in flight", e.limiter.InFlight())
		case <-ticker.C:
		}
	}
}

func (e *Executor) releaseJob(runID string) {
	e.mu.Lock()
	if _, ok := e.inflight[runID]; ok {
		delete(e.inflight, runID)
		e.limiter.Release()
		jobsInFlight.Set(float64(e.limiter.InFlight()))
	}
	e.mu.Unlock()
}

func mapAAPStatus(status string) workflow.RunStatus {
	switch status {
	case "successful":
		return workflow.RunCompleted
	case "failed", "error":
		return workflow.RunFailed
	case "canceled":
		return workflow.RunFailed
	case "pending", "waiting":
		return workflow.RunPending
	case "running":
		return workflow.RunRunning
	default:
		return workflow.RunPending
	}
}

var _ workflow.Executor = (*Executor)(nil)
