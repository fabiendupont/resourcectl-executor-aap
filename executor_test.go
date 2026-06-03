// Copyright 2025 The infractl Authors
// SPDX-License-Identifier: Apache-2.0

package aap

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/rs/zerolog"

	"github.com/fabiendupont/infractl/workflow"
)

var testLogger = zerolog.Nop()

func newTestClient(ts *httptest.Server) *Client {
	client, _ := NewClient(ClientConfig{
		BaseURL: ts.URL,
		Auth:    &BearerTokenAuth{Token: "test-token"},
	})
	return client
}

func TestExecutorSubmitJobTemplate(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}
		if r.URL.Path != "/api/v2/job_templates/provision-vm/launch/" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		if r.Header.Get("Authorization") != "Bearer test-token" {
			t.Errorf("missing or wrong auth header: %s", r.Header.Get("Authorization"))
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

	exec := NewExecutor(newTestClient(ts), testLogger, ExecutorConfig{})

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

	exec := NewExecutor(newTestClient(ts), testLogger, ExecutorConfig{})

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

	exec := NewExecutor(newTestClient(ts), testLogger, ExecutorConfig{})

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

	exec := NewExecutor(newTestClient(ts), testLogger, ExecutorConfig{})

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

	exec := NewExecutor(newTestClient(ts), testLogger, ExecutorConfig{})

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

func TestRequestIDCorrelation(t *testing.T) {
	var receivedID string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedID = r.Header.Get("X-Request-ID")
		json.NewEncoder(w).Encode(Job{ID: 1, Status: "successful"})
	}))
	defer ts.Close()

	client := newTestClient(ts)

	ctx := ContextWithRequestID(context.Background(), "req-abc-123")
	client.GetJob(ctx, "1")

	if receivedID != "req-abc-123" {
		t.Errorf("X-Request-ID = %q, want %q", receivedID, "req-abc-123")
	}
}

func TestOAuth2TokenRefresh(t *testing.T) {
	tokenCalls := 0
	tokenServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		tokenCalls++
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"access_token": "fresh-token",
			"expires_in":   3600,
			"token_type":   "Bearer",
		})
	}))
	defer tokenServer.Close()

	var receivedAuth string
	aapServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedAuth = r.Header.Get("Authorization")
		json.NewEncoder(w).Encode(Job{ID: 1, Status: "successful"})
	}))
	defer aapServer.Close()

	auth := NewOAuth2Auth(OAuth2Config{
		TokenURL:     tokenServer.URL,
		ClientID:     "my-client",
		ClientSecret: "my-secret",
	}, http.DefaultClient)

	client, err := NewClient(ClientConfig{
		BaseURL: aapServer.URL,
		Auth:    auth,
	})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}

	client.GetJob(context.Background(), "1")

	if receivedAuth != "Bearer fresh-token" {
		t.Errorf("Authorization = %q, want %q", receivedAuth, "Bearer fresh-token")
	}
	if tokenCalls != 1 {
		t.Errorf("token endpoint called %d times, want 1", tokenCalls)
	}

	client.GetJob(context.Background(), "1")
	if tokenCalls != 1 {
		t.Errorf("token endpoint called %d times after cache, want 1", tokenCalls)
	}
}

func TestRateLimiting(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(LaunchResponse{ID: 1})
	}))
	defer ts.Close()

	exec := NewExecutor(newTestClient(ts), testLogger, ExecutorConfig{MaxInFlight: 1})

	// First submit should work.
	handler := workflow.Handler{Ref: "test"}
	_, err := exec.Submit(context.Background(), handler, nil)
	if err != nil {
		t.Fatalf("first Submit: %v", err)
	}

	// Second submit should be rate limited (first job not released).
	_, err = exec.Submit(context.Background(), handler, nil)
	if err == nil {
		t.Fatal("expected rate limit error on second submit")
	}
}

func TestHealthCheck(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v2/ping/" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()

	exec := NewExecutor(newTestClient(ts), testLogger, ExecutorConfig{})

	if err := exec.Healthy(context.Background()); err != nil {
		t.Fatalf("Healthy: %v", err)
	}
}

func TestDrain(t *testing.T) {
	exec := NewExecutor(newTestClient(httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))), testLogger, ExecutorConfig{})

	// No in-flight jobs — drain should complete immediately.
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	if err := exec.Drain(ctx); err != nil {
		t.Fatalf("Drain: %v", err)
	}

	// After drain, submit should be rejected.
	_, err := exec.Submit(context.Background(), workflow.Handler{Ref: "test"}, nil)
	if err == nil {
		t.Fatal("expected submit to be rejected after drain")
	}
}
