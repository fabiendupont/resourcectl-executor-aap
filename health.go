// Copyright 2025 The infractl Authors
// SPDX-License-Identifier: Apache-2.0

package aap

import (
	"context"
	"fmt"
	"net/http"
)

// Healthy checks that the AAP Controller is reachable by hitting
// the API root. Returns nil if healthy, error otherwise.
func (c *Client) Healthy(ctx context.Context) error {
	url := fmt.Sprintf("%s/api/v2/ping/", c.baseURL)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return fmt.Errorf("creating health check request: %w", err)
	}
	if err := c.setHeaders(req); err != nil {
		return fmt.Errorf("setting auth for health check: %w", err)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("AAP health check failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("AAP health check returned HTTP %d", resp.StatusCode)
	}
	return nil
}
