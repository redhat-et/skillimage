# CLAUDE.md — OCI Skill Registry

## Overview

A framework-agnostic OCI-based registry for AI agent skills.
Stores actual skill content as OCI artifacts, manages lifecycle,
and enables discovery. Companion to
[agentoperations/agent-registry](https://github.com/agentoperations/agent-registry)
(metadata/governance layer).

## Build and test

```bash
make build       # Build skillctl to bin/
make test        # Run all tests
make lint        # Run golangci-lint
make fmt         # Format code
```

## Project structure

| Path | Description |
| ---- | ----------- |
| `cmd/skillctl/` | CLI entry point |
| `internal/cli/` | Cobra commands |
| `internal/handler/` | HTTP handlers |
| `internal/service/` | Business logic (lifecycle state machine) |
| `internal/store/` | Storage interface + SQLite |
| `internal/server/` | Router, middleware |
| `pkg/skillcard/` | SkillCard parse, validate, serialize |
| `pkg/oci/` | Pack/push/pull/inspect (oras-go) |
| `pkg/verify/` | Sigstore signature verification |
| `pkg/lifecycle/` | State machine, semver rules |
| `pkg/diff/` | Version comparison |
| `schemas/` | JSON Schema for SkillCard |
| `api/` | OpenAPI 3.1 spec |
| `deploy/` | Dockerfile, Kustomize overlays |
| `docs/` | Design specs, research, ADRs |

## Architecture

Library-first: core logic in `pkg/`, CLI and server are thin
consumers. Two operating modes: standalone CLI (pack/push/pull
against OCI registries, no server needed) and server mode
(lifecycle management, search, UI support via REST API).

## Key technologies

- **Go 1.25+**
- **oras-go** for OCI operations
- **Cobra/Viper** for CLI
- **chi** for HTTP router
- **SQLite** for metadata (Postgres swap path)
- **cosign** for signature verification
