# CLAUDE.md

## Project Context

resourcectl-executor-aap implements a workflow executor for Ansible Automation Platform (AAP). It plugs into resourcectl's `workflow.Executor` interface (Submit/Poll/Cancel) to dispatch provisioning jobs to AAP Controller and poll their status.

Go module: `github.com/fabiendupont/resourcectl-executor-aap`

Dependencies: resourcectl v0.7.0, prometheus/client_golang, rs/zerolog.

## Architecture

Single Go package (flat layout, no subdirectories). All files live at the module root.

| File | Purpose |
|---|---|
| `executor.go` | Executor implementing `workflow.Executor` (Submit/Poll/Cancel) |
| `client.go` | HTTP client for AAP Controller API |
| `auth.go` | BearerTokenAuth, OAuth2Auth credential providers |
| `transport.go` | TLS configuration for AAP connections |
| `retry.go` | RetryConfig with exponential backoff |
| `circuit.go` | CircuitConfig for circuit breaker pattern |
| `ratelimit.go` | Rate limiting for AAP API calls |
| `health.go` | Health check endpoint |
| `metrics.go` | Prometheus metrics instrumentation |
| `executor_test.go` | Tests |

## Build / Test / Lint

```bash
go build ./...              # Build
go test ./... -v -count=1   # Run tests
go vet ./...                # Static analysis
```

## Critical Rules

- Sign off all commits: `git commit -s`
- Add AI attribution trailer when AI-assisted:
  ```
  Assisted-by: Claude Code <noreply@anthropic.com>
  ```
- All AAP API calls must go through the resilience stack (retry, circuit breaker, rate limiter).
- Auth credentials (tokens, OAuth2 secrets) must never be logged or included in error messages.
