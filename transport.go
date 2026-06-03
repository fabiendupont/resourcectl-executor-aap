// Copyright 2025 The infractl Authors
// SPDX-License-Identifier: Apache-2.0

package aap

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"net/http"
	"os"
)

// TLSConfig holds TLS parameters for the AAP Controller connection.
type TLSConfig struct {
	CACert     string // path to CA certificate file
	ClientCert string // path to client certificate (for mTLS)
	ClientKey  string // path to client private key (for mTLS)
	Insecure   bool   // skip TLS verification (dev only)
}

// NewTLSTransport creates an http.Transport with the given TLS configuration.
func NewTLSTransport(cfg TLSConfig) (*http.Transport, error) {
	tlsCfg := &tls.Config{
		MinVersion: tls.VersionTLS12,
	}

	if cfg.Insecure {
		tlsCfg.InsecureSkipVerify = true
	}

	if cfg.CACert != "" {
		caCert, err := os.ReadFile(cfg.CACert)
		if err != nil {
			return nil, fmt.Errorf("reading CA cert %s: %w", cfg.CACert, err)
		}
		pool := x509.NewCertPool()
		if !pool.AppendCertsFromPEM(caCert) {
			return nil, fmt.Errorf("failed to parse CA cert %s", cfg.CACert)
		}
		tlsCfg.RootCAs = pool
	}

	if cfg.ClientCert != "" && cfg.ClientKey != "" {
		cert, err := tls.LoadX509KeyPair(cfg.ClientCert, cfg.ClientKey)
		if err != nil {
			return nil, fmt.Errorf("loading client cert: %w", err)
		}
		tlsCfg.Certificates = []tls.Certificate{cert}
	}

	return &http.Transport{TLSClientConfig: tlsCfg}, nil
}

// correlationTransport wraps an http.RoundTripper to inject a
// X-Request-ID header from context for audit trail correlation.
type correlationTransport struct {
	base http.RoundTripper
}

func (t *correlationTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if reqID := req.Context().Value(requestIDKey{}); reqID != nil {
		if id, ok := reqID.(string); ok && id != "" {
			req.Header.Set("X-Request-ID", id)
		}
	}
	return t.base.RoundTrip(req)
}

type requestIDKey struct{}

// ContextWithRequestID stores a request ID in the context for audit
// correlation across systems.
func ContextWithRequestID(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, requestIDKey{}, id)
}
