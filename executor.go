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

	"github.com/fabiendupont/infractl/workflow"
)

// Executor implements workflow.Executor by talking to AAP Controller.
type Executor struct {
	client *Client
}

// NewExecutor creates an AAP executor with the given client.
func NewExecutor(client *Client) *Executor {
	return &Executor{client: client}
}

// Submit launches an AAP Job Template (or Workflow Job Template) and
// returns a Run with the AAP job ID. The handler's Ref is the template
// name. Input is passed as extra_vars.
func (e *Executor) Submit(ctx context.Context, handler workflow.Handler, input map[string]interface{}) (*workflow.Run, error) {
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

	if err != nil {
		return nil, fmt.Errorf("launching AAP template %q: %w", handler.Ref, err)
	}

	return &workflow.Run{
		ID:     strconv.Itoa(resp.ID),
		Status: workflow.RunPending,
	}, nil
}

// Poll checks the status of an AAP job and maps it to a workflow.Run.
func (e *Executor) Poll(ctx context.Context, runID string) (*workflow.Run, error) {
	job, err := e.client.GetJob(ctx, runID)
	if err != nil {
		return nil, err
	}

	run := &workflow.Run{
		ID:     strconv.Itoa(job.ID),
		Status: mapAAPStatus(job.Status),
	}

	if job.ResultTraceback != "" {
		run.Error = job.ResultTraceback
	}

	return run, nil
}

// Cancel cancels a running AAP job.
func (e *Executor) Cancel(ctx context.Context, runID string) error {
	return e.client.CancelJob(ctx, runID)
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
