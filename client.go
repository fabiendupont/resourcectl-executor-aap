// Copyright 2025 The infractl Authors
// SPDX-License-Identifier: Apache-2.0

package aap

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// ClientConfig holds the AAP Controller connection parameters.
type ClientConfig struct {
	BaseURL string
	Auth    AuthMethod
	TLS     TLSConfig
	Timeout time.Duration
	Retry   *RetryConfig
	Circuit *CircuitConfig
}

// Client talks to the AAP Controller REST API.
type Client struct {
	baseURL    string
	httpClient *http.Client
	auth       AuthMethod
}

// NewClient creates an AAP Controller client with TLS and auth configured.
func NewClient(cfg ClientConfig) (*Client, error) {
	timeout := cfg.Timeout
	if timeout == 0 {
		timeout = 30 * time.Second
	}

	var transport http.RoundTripper = http.DefaultTransport

	if cfg.TLS.CACert != "" || cfg.TLS.ClientCert != "" || cfg.TLS.Insecure {
		t, err := NewTLSTransport(cfg.TLS)
		if err != nil {
			return nil, fmt.Errorf("configuring TLS: %w", err)
		}
		transport = t
	}

	// Chain transports: TLS → circuit breaker → retry → correlation
	if cfg.Circuit != nil {
		transport = WithCircuitBreaker(transport, *cfg.Circuit)
	}
	if cfg.Retry != nil {
		transport = WithRetry(transport, *cfg.Retry)
	}
	transport = &correlationTransport{base: transport}

	return &Client{
		baseURL:    cfg.BaseURL,
		auth:       cfg.Auth,
		httpClient: &http.Client{Timeout: timeout, Transport: transport},
	}, nil
}

// LaunchRequest is the payload for launching a job or workflow template.
type LaunchRequest struct {
	ExtraVars map[string]interface{} `json:"extra_vars,omitempty"`
}

// LaunchResponse is the response from a template launch.
type LaunchResponse struct {
	ID   int    `json:"id"`
	Type string `json:"type"`
	URL  string `json:"url"`
}

// Job represents an AAP job status response.
type Job struct {
	ID              int    `json:"id"`
	Status          string `json:"status"`
	Failed          bool   `json:"failed"`
	Started         string `json:"started"`
	Finished        string `json:"finished"`
	ResultTraceback string `json:"result_traceback"`
}

// LaunchJobTemplate launches a job template by name.
func (c *Client) LaunchJobTemplate(ctx context.Context, templateName string, req LaunchRequest) (*LaunchResponse, error) {
	url := fmt.Sprintf("%s/api/v2/job_templates/%s/launch/", c.baseURL, templateName)
	return c.launch(ctx, url, req)
}

// LaunchWorkflowTemplate launches a workflow job template by name.
func (c *Client) LaunchWorkflowTemplate(ctx context.Context, templateName string, req LaunchRequest) (*LaunchResponse, error) {
	url := fmt.Sprintf("%s/api/v2/workflow_job_templates/%s/launch/", c.baseURL, templateName)
	return c.launch(ctx, url, req)
}

// GetJob retrieves the status of a job by ID.
func (c *Client) GetJob(ctx context.Context, jobID string) (*Job, error) {
	url := fmt.Sprintf("%s/api/v2/jobs/%s/", c.baseURL, jobID)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	if err := c.setHeaders(req); err != nil {
		return nil, err
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("getting job %s: %w", jobID, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("getting job %s: HTTP %d: %s", jobID, resp.StatusCode, string(body))
	}

	var job Job
	if err := json.NewDecoder(resp.Body).Decode(&job); err != nil {
		return nil, fmt.Errorf("decoding job response: %w", err)
	}
	return &job, nil
}

// CancelJob cancels a running job.
func (c *Client) CancelJob(ctx context.Context, jobID string) error {
	url := fmt.Sprintf("%s/api/v2/jobs/%s/cancel/", c.baseURL, jobID)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, nil)
	if err != nil {
		return err
	}
	if err := c.setHeaders(req); err != nil {
		return err
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("canceling job %s: %w", jobID, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusAccepted {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("canceling job %s: HTTP %d: %s", jobID, resp.StatusCode, string(body))
	}
	return nil
}

func (c *Client) launch(ctx context.Context, url string, launchReq LaunchRequest) (*LaunchResponse, error) {
	body, err := json.Marshal(launchReq)
	if err != nil {
		return nil, fmt.Errorf("marshaling launch request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	if err := c.setHeaders(req); err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("launching template: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("launching template: HTTP %d: %s", resp.StatusCode, string(respBody))
	}

	var launchResp LaunchResponse
	if err := json.NewDecoder(resp.Body).Decode(&launchResp); err != nil {
		return nil, fmt.Errorf("decoding launch response: %w", err)
	}
	return &launchResp, nil
}

func (c *Client) setHeaders(req *http.Request) error {
	if c.auth != nil {
		if err := c.auth.SetAuth(req); err != nil {
			return fmt.Errorf("setting auth: %w", err)
		}
	}
	req.Header.Set("Accept", "application/json")
	return nil
}
