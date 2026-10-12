# Smart Server Monitor

A Raspberry Pi controlled prototype that monitors a primary laptop, wakes a standby laptop with Wake-on-LAN, checks application health, and changes traffic routing. See [the project idea](docs/idea.md) for the full design.

## Tech stack

- **Backend and device agents:** Go. The controller/API, laptop metrics agent, and demo application are planned in Go.
- **Frontend:** Next.js with TypeScript. Node.js runs the Next.js dashboard; Go remains the only monitoring and scaling backend.
- **Routing:** HAProxy on the Pi.
- **Storage:** SQLite for timestamped metrics; controller events are planned.
- **Deployment:** Docker Compose for the laptop app; systemd for controller and agent services.
- **Network control:** Magic-packet wake over the local network. The current laptop reports Wi-Fi WoWLAN capability; an actual wake trial is still pending.

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
| `web/` | Raspberry Pi | Planned Next.js dashboard |
| `config/example.yaml` | Raspberry Pi | Example topology and policy values |
| `deploy/` | Pi and laptops | HAProxy, Docker, and systemd setup |

The Go agent can read live Linux laptop metrics. The Go controller polls one agent every five seconds and stores samples locally. Scaling and physical hardware integration remain future phases.

## Run locally

Start each Go service in its own terminal from the repository root:

```sh
go run ./cmd/controller
go run ./cmd/agent
go run ./cmd/demoapp
```

The controller listens on `127.0.0.1:8080`, agent on `127.0.0.1:8081`, and demo app on `127.0.0.1:8000`. Use `CONTROLLER_ADDR`, `AGENT_ADDR`, or `DEMOAPP_ADDR` to change a bind address. To start the dashboard, run `npm install` and `npm run dev` in `web/`; it opens at `http://127.0.0.1:3000`.

For a laptop-only metrics trial, start the agent and controller in that order, then read:

```sh
curl http://127.0.0.1:8081/metrics
curl http://127.0.0.1:8080/api/status
curl 'http://127.0.0.1:8080/api/metrics?limit=10'
```

Start `go run ./cmd/demoapp` in a third terminal to see `app_health.status` become `healthy`; without it, the controller reports the app as `unhealthy` while laptop metrics can stay available. Set `AGENT_URL` and `APP_URL` on the controller to the services' private LAN URLs when running on separate machines. Set `SQLITE_PATH` to change the database path (default `data/monitor.sqlite3`) or `METRICS_DISK_PATH` on the agent to measure another mounted disk (default `/`). Recent samples remain in SQLite after controller restarts. When the agent is unreachable, the API sets `metrics_available` to false; old samples remain historical readings. The agent reads Linux `/proc`, so this implementation runs on Linux laptops and the Raspberry Pi.

## Current service boundaries

| Service | Available endpoints | Responsibility |
| --- | --- | --- |
| Go controller | `GET /health`, `GET /api/status`, `GET /api/metrics`, `GET /api/metrics/latest` | Polls one agent and the demo app independently; stores recent metrics; reports `UNCONFIGURED` until hardware integration |
| Go agent | `GET /health`, `GET /identity`, `GET /metrics` | Reads CPU, memory, disk, network counters, uptime, and optional temperature |
| Go demo app | `GET /health`, `GET /`, `GET /workload` | Stateless app that identifies the serving host and provides bounded CPU work |
| Next.js dashboard | `GET /` | Reads the controller API on the server; browser does not need the private Go address |

The controller will later own Wake-on-LAN, scaling decisions, events, and HAProxy changes. Next.js is only the frontend. An unavailable controller appears as unavailable on the dashboard rather than as a healthy machine.

## Build targets

These commands build the Go services separately. The Raspberry Pi target assumes a 64-bit Linux OS; the current laptop is Linux amd64.

```sh
CGO_ENABLED=1 go build -o bin/controller ./cmd/controller
go build -o bin/agent ./cmd/agent
go build -o bin/demoapp ./cmd/demoapp
```

The dashboard's production build is `npm run build` inside `web/`. Keep the generated `web/package-lock.json` in version control so a later `npm ci` installs the same versions.

The SQLite driver currently uses CGO. Build the controller natively on the Pi with a C compiler, or use an ARM64 cross compiler. The agent and demo app remain ordinary Go binaries.

## Application traffic path (in progress)

The [laptop Compose file](deploy/docker/README.md) builds the same Go app image for both laptops. The [HAProxy template](deploy/haproxy/README.md) starts with only the primary enabled and checks each app's `/health`. The Pi and final laptop addresses are not available yet, so the proxy path remains to be exercised on the hardware.

## Build order

1. Confirm the standby wakes from the intended power state and starts its app automatically.
2. Run the same stateless app on both laptops through the Pi proxy.
3. Add metrics, health checks, and the activation state machine.
4. Add event history, dashboard, alerts, and a repeatable demo.

Copy `config/example.yaml` to `config/local.yaml` when device addresses and the standby MAC are known. Keep real settings and secrets out of version control.
