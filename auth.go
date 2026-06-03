// Copyright 2025 The infractl Authors
// SPDX-License-Identifier: Apache-2.0

package aap

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

// AuthMethod defines how the executor authenticates with AAP Controller.
type AuthMethod interface {
	SetAuth(req *http.Request) error
}

// BearerTokenAuth uses a static Bearer token. Simplest method —
// suitable for service accounts with long-lived tokens.
type BearerTokenAuth struct {
	Token string
}

func (a *BearerTokenAuth) SetAuth(req *http.Request) error {
	req.Header.Set("Authorization", "Bearer "+a.Token)
	return nil
}

// OAuth2Config holds OAuth2 client credentials parameters for
// token acquisition and refresh.
type OAuth2Config struct {
	TokenURL     string
	ClientID     string
	ClientSecret string
	Scopes       []string
}

// OAuth2Auth uses the OAuth2 client credentials flow. Tokens are
// cached and automatically refreshed before expiry.
type OAuth2Auth struct {
	config     OAuth2Config
	httpClient *http.Client

	mu      sync.Mutex
	token   string
	expiry  time.Time
}

// NewOAuth2Auth creates an OAuth2 client credentials authenticator.
func NewOAuth2Auth(cfg OAuth2Config, httpClient *http.Client) *OAuth2Auth {
	return &OAuth2Auth{
		config:     cfg,
		httpClient: httpClient,
	}
}

func (a *OAuth2Auth) SetAuth(req *http.Request) error {
	token, err := a.getToken(req.Context())
	if err != nil {
		return fmt.Errorf("obtaining OAuth2 token: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	return nil
}

func (a *OAuth2Auth) getToken(ctx context.Context) (string, error) {
	a.mu.Lock()
	defer a.mu.Unlock()

	// Refresh 30 seconds before expiry.
	if a.token != "" && time.Now().Add(30*time.Second).Before(a.expiry) {
		return a.token, nil
	}

	return a.refreshToken(ctx)
}

func (a *OAuth2Auth) refreshToken(ctx context.Context) (string, error) {
	data := url.Values{
		"grant_type":    {"client_credentials"},
		"client_id":     {a.config.ClientID},
		"client_secret": {a.config.ClientSecret},
	}
	if len(a.config.Scopes) > 0 {
		data.Set("scope", strings.Join(a.config.Scopes, " "))
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, a.config.TokenURL,
		strings.NewReader(data.Encode()))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := a.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("token request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("token endpoint returned HTTP %d", resp.StatusCode)
	}

	var tokenResp struct {
		AccessToken string `json:"access_token"`
		ExpiresIn   int    `json:"expires_in"`
		TokenType   string `json:"token_type"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&tokenResp); err != nil {
		return "", fmt.Errorf("decoding token response: %w", err)
	}

	a.token = tokenResp.AccessToken
	a.expiry = time.Now().Add(time.Duration(tokenResp.ExpiresIn) * time.Second)
	return a.token, nil
}
