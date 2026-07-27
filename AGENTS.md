# AGENTS.md

## Project

resourcectl-executor-aap is a workflow executor for Ansible Automation Platform (AAP). It implements `workflow.Executor` from resourcectl, forwarding Submit/Poll/Cancel operations to AAP Controller's REST API.

Single flat Go package — no subdirectories.

## Build and test

```bash
go build ./...
go vet ./...
go test ./... -v -count=1
```

## Code style

- Go with strict error checking
- No comments unless the "why" is non-obvious
- Match existing patterns

## Files

- `executor.go` — `Executor` implementing `workflow.Executor` (Submit/Poll/Cancel)
- `client.go` — HTTP client for AAP Controller API
- `auth.go` — `BearerTokenAuth`, `OAuth2Auth` (token URL, client ID/secret)
- `transport.go` — TLS configuration (CA cert, client cert, insecure mode)
- `retry.go` — `RetryConfig` with exponential backoff and jitter
- `circuit.go` — `CircuitConfig` for circuit breaker (threshold, timeout, half-open)
- `ratelimit.go` — per-client rate limiting
- `health.go` — health check against AAP Controller
- `metrics.go` — Prometheus metrics for executor operations
- `executor_test.go` — tests with httptest mock AAP server

## Dependencies

- `github.com/fabiendupont/resourcectl` v0.7.0 — only imports `workflow.Executor`, `workflow.Run`, `workflow.Handler`
- `github.com/prometheus/client_golang` — metrics
- `github.com/rs/zerolog` — logging

## Conventions

- Sign off all commits: `git commit -s`
- Never store credentials in code — use env vars or config
- The executor is resilient by default: retry + circuit breaker + rate limiting
