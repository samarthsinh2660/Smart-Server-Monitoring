# Smart Server Monitor

A Raspberry Pi controlled prototype that monitors a primary laptop, wakes a standby laptop with Wake-on-LAN, checks application health, and changes traffic routing. See [the project idea](docs/idea.md) for the full design.

## Tech stack

- **Backend and device agents:** Go. The controller/API, laptop metrics agent, and demo application are planned in Go.
- **Frontend:** Next.js with TypeScript. Node.js runs the Next.js dashboard; Go remains the only monitoring and scaling backend.
- **Routing:** HAProxy on the Pi.
- **Storage:** SQLite on the Pi for metrics and events.
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

The first software foundation is in place. The Go services expose local status and identity endpoints; the Next.js dashboard reads the Go controller status. Hardware monitoring and scaling are still future phases.

## Run locally

Start each Go service in its own terminal from the repository root:

```sh
go run ./cmd/controller
go run ./cmd/agent
go run ./cmd/demoapp
```

The controller listens on `127.0.0.1:8080`, agent on `127.0.0.1:8081`, and demo app on `127.0.0.1:8000`. Use `CONTROLLER_ADDR`, `AGENT_ADDR`, or `DEMOAPP_ADDR` to change a bind address. To start the dashboard, run `npm install` and `npm run dev` in `web/`; it opens at `http://127.0.0.1:3000`.

## Current service boundaries

| Service | Available endpoints | Responsibility |
| --- | --- | --- |
| Go controller | `GET /health`, `GET /api/status` | Private control API; reports `UNCONFIGURED` until hardware integration |
| Go agent | `GET /health`, `GET /identity` | Laptop process; metrics arrive in Phase 4 |
| Go demo app | `GET /health`, `GET /` | Stateless app that identifies the serving host |
| Next.js dashboard | `GET /` | Reads the controller API on the server; browser does not need the private Go address |

The controller will later own Wake-on-LAN, scaling decisions, SQLite, and HAProxy changes. Next.js is only the frontend. An unavailable controller appears as unavailable on the dashboard rather than as a healthy machine.

## Build targets

These commands build the Go services separately. The Raspberry Pi target assumes a 64-bit Linux OS; the current laptop is Linux amd64.

```sh
go build -o bin/controller ./cmd/controller
go build -o bin/agent ./cmd/agent
go build -o bin/demoapp ./cmd/demoapp
CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -o bin/pi/controller ./cmd/controller
```

The dashboard's production build is `npm run build` inside `web/`. Keep the generated `web/package-lock.json` in version control so a later `npm ci` installs the same versions.

## Build order

1. Confirm the standby wakes from the intended power state and starts its app automatically.
2. Run the same stateless app on both laptops through the Pi proxy.
3. Add metrics, health checks, and the activation state machine.
4. Add event history, dashboard, alerts, and a repeatable demo.

Copy `config/example.yaml` to `config/local.yaml` when device addresses and the standby MAC are known. Keep real settings and secrets out of version control.
