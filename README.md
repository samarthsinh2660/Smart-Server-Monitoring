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
| `internal/controller/` | Raspberry Pi | Metrics polling and independent app health checks; later, scaling decisions |
| `internal/metrics/` | Pi and laptops | Metrics collection and transport |
| `internal/config/` | Pi and laptops | Configuration loading |
| `internal/proxy/` | Raspberry Pi | HAProxy target management |
| `internal/store/` | Raspberry Pi | SQLite metrics; event storage planned |
| `web/` | Raspberry Pi | Next.js status dashboard |
| `config/example.yaml` | Raspberry Pi | Example topology and policy values |
| `deploy/` | Pi and laptops | HAProxy, Docker, and systemd setup |

The Go agent can read live Linux laptop metrics. The Go controller polls one agent every five seconds and stores samples locally. Scaling and physical hardware integration remain future phases.

## Run the laptop prototype

You need a Linux laptop, Go 1.22 or newer, and a C compiler for the controller's SQLite driver. Run each long-lived service in a **separate terminal** from the repository root. Leave those terminals open while checking the API.

| Terminal | Command | Default address |
| --- | --- | --- |
| 1: laptop metrics agent | `go run ./cmd/agent` | `127.0.0.1:8081` |
| 2: demo application | `go run ./cmd/demoapp` | `127.0.0.1:8000` |
| 3: controller | `go run ./cmd/controller` | `127.0.0.1:8080` |

Start the agent and app before the controller. Wait about five seconds for its first checks. In a **fourth terminal**, run:

```sh
curl http://127.0.0.1:8081/metrics
curl http://127.0.0.1:8000/health
curl http://127.0.0.1:8080/api/status
curl 'http://127.0.0.1:8080/api/metrics?limit=10'
curl 'http://127.0.0.1:8080/api/traffic?limit=10'
curl 'http://127.0.0.1:8080/api/events?limit=10'
```

The status response should contain `"metrics_available":true` and `"app_health":{"status":"healthy",...}`. `"state":"UNCONFIGURED"` and `"hardware_ready":false` are expected until the Pi and wake/routing hardware are connected. Sample times are UTC; network byte values are cumulative counters. `/api/metrics` returns newest samples first, and `/api/metrics/latest` returns the newest one. The dashboard also shows metrics availability and app health: run `npm ci` once in `web/`, then `npm run dev` there and open `http://127.0.0.1:3000`.

`/api/traffic` records app request counts, HTTP 4xx/5xx counts, request rate, error percentage, and average response time. The first sample has no rates because it needs a previous reading. Generate requests after the controller has run for five seconds, then read `/api/traffic` again. The demo app's `/health` and `/stats` probes are excluded from traffic counts. `/api/events` lists changes in app health, metrics availability, and traffic-stats availability; it is stored in the same SQLite file. To exercise traffic, run this in the fourth terminal:

```sh
for i in {1..20}; do curl -s 'http://127.0.0.1:8000/workload?duration_ms=500' >/dev/null; done
curl -i 'http://127.0.0.1:8000/workload?duration_ms=0'
curl http://127.0.0.1:8000/stats
curl 'http://127.0.0.1:8080/api/traffic?limit=5'
curl 'http://127.0.0.1:8080/api/events?limit=10'
```

The invalid `duration_ms=0` request returns HTTP 400 and increments the app error count. Requests per second and average latency use differences between consecutive five-second snapshots, so wait for the next controller poll if the newest sample still shows an earlier interval.

Stop a service with Ctrl+C in its terminal. Restart the controller after changing its code. The SQLite file defaults to `data/monitor.sqlite3` and is ignored by Git; saved samples survive a controller restart. If `curl` says “Couldn't connect to server,” check that the corresponding service is still running in its own terminal. If the app is stopped, `app_health.status` becomes `unhealthy` while metrics can remain available; if the agent is stopped, `metrics_available` becomes false while the app can remain healthy. Historical samples stay in the database in both cases.

For different hosts, set `AGENT_ADDR` and `DEMOAPP_ADDR` to private LAN bind addresses on the laptops, then set `AGENT_URL` and `APP_URL` on the controller to those hosts' URLs. `CONTROLLER_ADDR` changes the controller bind address; `SQLITE_PATH` changes the database path; `METRICS_DISK_PATH` changes the measured disk (default `/`). `config/example.yaml` records planned network values but is not loaded by the running services yet. Do not expose these prototype endpoints to the public internet.

## Current service boundaries

| Service | Available endpoints | Responsibility |
| --- | --- | --- |
| Go controller | `GET /health`, `GET /api/status`, `GET /api/metrics`, `GET /api/metrics/latest`, `GET /api/traffic`, `GET /api/events` | Polls one agent, app health, and app stats independently; stores samples and events |
| Go agent | `GET /health`, `GET /identity`, `GET /metrics` | Reads CPU, memory, disk, network counters, uptime, and optional temperature |
| Go demo app | `GET /health`, `GET /`, `GET /workload`, `GET /stats` | Stateless app with bounded CPU work and cumulative request counters |
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
3. Extend the working single-laptop metrics and app checks to both laptops, then add the activation state machine.
4. Add event history, dashboard, alerts, and a repeatable demo.

Copy `config/example.yaml` to `config/local.yaml` when device addresses and the standby MAC are known. Keep real settings and secrets out of version control.
