# Smart Server Monitor

A Raspberry Pi controlled prototype that monitors a primary laptop, wakes a standby laptop with Wake-on-LAN, checks application health, and changes traffic routing. See [the project idea](docs/idea.md) for the full design.

## Tech stack

- **Backend and device agents:** Go. Separate binaries for the Pi controller/API, laptop metrics agent, and stateless demo application.
- **Frontend:** TypeScript, HTML, and CSS built with Vite to static browser files served by the Pi; no Node.js server is required on the Pi.
- **Routing:** HAProxy on the Pi.
- **Storage:** SQLite on the Pi for metrics and events.
- **Deployment:** Docker Compose for the laptop app; systemd for controller and agent services.
- **Network control:** Wake-on-LAN over the wired LAN.

## Repository layout

| Path | Runs on | Responsibility |
| --- | --- | --- |
| `cmd/controller/` | Raspberry Pi | Controller and HTTP API entry point |
| `cmd/agent/` | Both laptops | Metrics agent entry point |
| `cmd/demoapp/` | Both laptops | Stateless application entry point |
| `internal/controller/` | Raspberry Pi | Decision state machine and orchestration |
| `internal/metrics/` | Pi and laptops | Metrics collection and transport |
| `internal/config/` | Pi and laptops | Configuration loading |
| `internal/proxy/` | Raspberry Pi | HAProxy target management |
| `internal/store/` | Raspberry Pi | SQLite events and metrics |
| `web/` | Built for browser | Dashboard TypeScript, HTML, and CSS |
| `config/example.yaml` | Raspberry Pi | Example topology and policy values |
| `deploy/` | Pi and laptops | HAProxy, Docker, and systemd setup |

This is a repository scaffold. The controller, agent, application, and dashboard behavior will be implemented next.

## Build order

1. Confirm the standby wakes from the intended power state and starts its app automatically.
2. Run the same stateless app on both laptops through the Pi proxy.
3. Add metrics, health checks, and the activation state machine.
4. Add event history, dashboard, alerts, and a repeatable demo.

Copy `config/example.yaml` to `config/local.yaml` when device addresses and the standby MAC are known. Keep real settings and secrets out of version control.
