# Go Project Scaffold Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Create a repository structure for a Go Raspberry Pi controller, Go laptop metrics agents, Go demo application, and static TypeScript dashboard.

**Architecture:** The Pi runs one Go controller/API and HAProxy. Both laptops run the same Go demo app and a Go metrics agent after boot. The TypeScript dashboard is built to static assets served by the Pi.

**Tech Stack:** Go, vanilla TypeScript, Vite at build time, HTML/CSS, HAProxy, SQLite, Docker Compose, systemd.

## Global Constraints

- Keep the Pi controller as the sole owner of activation decisions.
- Route traffic only to an application instance that passed health checks.
- Limit the first version to one primary and one standby.
- Use example LAN addresses and a placeholder MAC address in configuration.
- This task creates structure and documentation; runtime behavior is a later task.

---

## File map

| Path | Responsibility |
| --- | --- |
| `README.md` | State the selected stack and build order |
| `.gitignore` | Exclude build output, local configuration, and runtime data |
| `go.mod` | Name the Go module and language floor |
| `config/example.yaml` | Show topology and controller policy settings |
| `cmd/controller/` | Pi controller and API entry point |
| `cmd/agent/` | Laptop metrics agent entry point |
| `cmd/demoapp/` | Stateless demo app entry point |
| `internal/controller/` | Activation policy and state machine |
| `internal/metrics/` | Metrics collection and polling |
| `internal/config/` | Configuration loading and validation |
| `internal/proxy/` | HAProxy target management |
| `internal/store/` | SQLite metric and event persistence |
| `web/` | Static TypeScript dashboard source |
| `deploy/` | HAProxy, Docker, and systemd setup notes |

### Task 1: Repository metadata

**Files:** `README.md`, `.gitignore`, `go.mod`.

- [x] Select Go for backend, agents, and demo app; TypeScript for the browser.
- [x] Define the Go module and exclude generated or machine-specific files.
- [x] Record the first hardware proof and build order.

### Task 2: Go package and configuration folders

**Files:** `cmd/`, `internal/`, and `config/example.yaml`.

- [x] Separate executable entry points from internal controller, metrics, config, proxy, and store packages.
- [x] Include example hosts, standby MAC, ports, polling interval, load threshold, health failure count, and startup timeout.
- [x] Keep the example free of credentials and real network identifiers.

### Task 3: Frontend and deployment folders

**Files:** `web/README.md` and README files in each `deploy` subfolder.

- [x] Explain that Vite builds static assets and Node.js is not a Pi runtime dependency.
- [x] Explain that the standby app starts on boot and joins the proxy only after health succeeds.
- [x] Record where HAProxy, Docker Compose, and systemd setup belongs.

## Next implementation sequence

1. Prove WoL and automatic standby app startup.
2. Serve a Go demo app through the Pi proxy.
3. Add laptop metrics and application health polling.
4. Add controller state transitions and event persistence.
5. Add the TypeScript dashboard, alerts, and a repeatable demo workload.
