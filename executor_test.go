// Copyright 2025 The infractl Authors
// SPDX-License-Identifier: Apache-2.0

package aap

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/fabiendupont/infractl/workflow"
)

func TestExecutorSubmitJobTemplate(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}
		if r.URL.Path != "/api/v2/job_templates/provision-vm/launch/" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}

		var req LaunchRequest
		json.NewDecoder(r.Body).Decode(&req)
		if req.ExtraVars["name"] != "test-vm" {
			t.Errorf("extra_vars[name] = %v, want test-vm", req.ExtraVars["name"])
		}

		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(LaunchResponse{ID: 42})
	}))
	defer ts.Close()

	client := NewClient(ClientConfig{BaseURL: ts.URL, Token: "test-token"})
	exec := NewExecutor(client)

	handler := workflow.Handler{Ref: "provision-vm"}
	run, err := exec.Submit(context.Background(), handler, map[string]interface{}{"name": "test-vm"})
	if err != nil {
		t.Fatalf("Submit: %v", err)
	}
	if run.ID != "42" {
		t.Errorf("Run.ID = %q, want %q", run.ID, "42")
	}
	if run.Status != workflow.RunPending {
		t.Errorf("Run.Status = %q, want %q", run.Status, workflow.RunPending)
	}
}

func TestExecutorSubmitWorkflowTemplate(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v2/workflow_job_templates/full-provision/launch/" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(LaunchResponse{ID: 99})
	}))
	defer ts.Close()

	client := NewClient(ClientConfig{BaseURL: ts.URL, Token: "test-token"})
	exec := NewExecutor(client)

	handler := workflow.Handler{
		Ref:      "full-provision",
		Metadata: map[string]string{"template_type": "workflow"},
	}
	run, err := exec.Submit(context.Background(), handler, nil)
	if err != nil {
		t.Fatalf("Submit: %v", err)
	}
	if run.ID != "99" {
		t.Errorf("Run.ID = %q, want %q", run.ID, "99")
	}
}

func TestExecutorPoll(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v2/jobs/42/" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		json.NewEncoder(w).Encode(Job{ID: 42, Status: "successful"})
	}))
	defer ts.Close()

	client := NewClient(ClientConfig{BaseURL: ts.URL, Token: "test-token"})
	exec := NewExecutor(client)

	run, err := exec.Poll(context.Background(), "42")
	if err != nil {
		t.Fatalf("Poll: %v", err)
	}
	if run.Status != workflow.RunCompleted {
		t.Errorf("Run.Status = %q, want %q", run.Status, workflow.RunCompleted)
	}
}

func TestExecutorPollFailed(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		json.NewEncoder(w).Encode(Job{ID: 42, Status: "failed", ResultTraceback: "task failed"})
	}))
	defer ts.Close()

	client := NewClient(ClientConfig{BaseURL: ts.URL, Token: "test-token"})
	exec := NewExecutor(client)

	run, err := exec.Poll(context.Background(), "42")
	if err != nil {
		t.Fatalf("Poll: %v", err)
	}
	if run.Status != workflow.RunFailed {
		t.Errorf("Run.Status = %q, want %q", run.Status, workflow.RunFailed)
	}
	if run.Error != "task failed" {
		t.Errorf("Run.Error = %q, want %q", run.Error, "task failed")
	}
}

func TestExecutorCancel(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}
		if r.URL.Path != "/api/v2/jobs/42/cancel/" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		w.WriteHeader(http.StatusAccepted)
	}))
	defer ts.Close()

	client := NewClient(ClientConfig{BaseURL: ts.URL, Token: "test-token"})
	exec := NewExecutor(client)

	if err := exec.Cancel(context.Background(), "42"); err != nil {
		t.Fatalf("Cancel: %v", err)
	}
}

func TestMapAAPStatus(t *testing.T) {
	tests := []struct {
		aap    string
		expect workflow.RunStatus
	}{
		{"successful", workflow.RunCompleted},
		{"failed", workflow.RunFailed},
		{"error", workflow.RunFailed},
		{"canceled", workflow.RunFailed},
		{"pending", workflow.RunPending},
		{"waiting", workflow.RunPending},
		{"running", workflow.RunRunning},
		{"unknown", workflow.RunPending},
	}

	for _, tt := range tests {
		got := mapAAPStatus(tt.aap)
		if got != tt.expect {
			t.Errorf("mapAAPStatus(%q) = %q, want %q", tt.aap, got, tt.expect)
		}
	}
}
