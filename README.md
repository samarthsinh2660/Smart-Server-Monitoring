# Smart Server Monitor

A Raspberry Pi controlled prototype that monitors a primary laptop, wakes a standby laptop with Wake-on-LAN, checks application health, and changes traffic routing. See [the project idea](docs/idea.md) for the full design.

## Tech stack

- **Backend and device agents:** Go. The controller/API, laptop metrics agent, and demo application are planned in Go.
- **Frontend:** Next.js with TypeScript. Node.js runs the Next.js dashboard; Go remains the only monitoring and scaling backend.
- **Routing:** HAProxy on the Pi.
- **Storage:** SQLite for timestamped metrics and controller events.
- **AI preview:** Python demand forecasting and surge classification trained on the real Azure Functions 2019 trace. The Go controller forwards its local scoring service result.
- **Deployment:** Docker Compose for the laptop app; systemd for controller and agent services.
- **Network control:** Magic-packet wake over the local network. The current laptop reports Wi-Fi WoWLAN capability; an actual wake trial is still pending.

## Repository layout

| Path | Runs on | Responsibility |
| --- | --- | --- |
| `cmd/controller/` | Raspberry Pi | Controller and HTTP API entry point |
| `cmd/agent/` | Both laptops | Metrics agent entry point |
| `cmd/demoapp/` | Both laptops | Stateless application entry point |
| `demo/traffic-lab/` | Development laptop | Bounded traffic generator for dashboard demonstrations |
| `model/` | Development laptop and later Pi | Python training, evaluation, artifact, and local inference service |
| `internal/controller/` | Raspberry Pi | Metrics polling and independent app health checks; later, scaling decisions |
| `internal/metrics/` | Pi and laptops | Metrics collection and transport |
| `internal/config/` | Pi and laptops | Configuration loading |
| `internal/proxy/` | Raspberry Pi | HAProxy target management |
| `internal/store/` | Raspberry Pi | SQLite metrics, traffic, and controller events |
| `web/` | Laptop 1 | Next.js dashboard; reads the Pi API over the LAN |
| `start-backend.sh` | Development laptop | Build and start the Go services and the trained Python model together |
| `config/example.yaml` | Raspberry Pi | Example topology and policy values |
| `deploy/` | Pi and laptops | HAProxy, Docker, and systemd setup |

The Go agent reads native Windows and Linux laptop metrics. The controller polls the primary and an optional standby every five seconds, stores separate histories, and exposes both servers through its API. [Traffic Lab](demo/traffic-lab/README.md) supports presets and bounded custom rate, concurrency, work cost, and duration. The activation manager implements sustained CPU activation, optional validated AI activation, Wi-Fi magic packets, health-gated HAProxy control, failure handling, and guarded reset. Hardware control is disabled by default; actual Windows/Pi/wireless trials remain to be completed.

The deployment uses a Raspberry Pi 4 Model B for the controller, model inference, SQLite, and HAProxy, with native Windows applications and a dashboard hosted on Laptop 1. The default policy warns at 60% primary CPU, requests activation after 70% continuously for 15 seconds, and permits explicitly validated AI to request an earlier wake. Follow the [Windows and Pi setup guide](docs/windows-pi-setup.md) for startup commands, configuration, and physical acceptance checks; the [deployment design](docs/windows-wireless-deployment.md) explains the boundaries.

Build Windows binaries from Linux with `./build-windows.sh`. On Windows, use `start-windows.ps1 -Role standby -SkipBuild` for Laptop 2, and `start-windows.ps1 -Role primary -Presentation -PiAddress <Pi-IP> -SkipBuild` for Laptop 1. On the Pi, configure `config/pi.env` and use `./start-pi.sh`. The existing `./start-backend.sh` remains the single-machine development launcher.

Continuing with Codex after switching to Windows: read [the Windows Codex handoff](docs/windows-codex-handoff.md) for current implementation evidence, transfer instructions, remaining tests, and a continuation prompt.

## Run the laptop prototype

You need a Linux laptop, Go 1.22 or newer, and a C compiler for the controller's SQLite driver. From the repository root, start the agent, demo app, Traffic Lab, and controller with one command:

```sh
./start-backend.sh
```

Leave that terminal open. Ctrl+C stops the services. The script builds the Go binaries into ignored `bin/` files before starting them and starts the Python model service when its artifact exists. Stop any older copies first, or their ports will already be in use.

For individual debugging, run each service in its own terminal:

| Terminal | Command | Default address |
| --- | --- | --- |
| 1: laptop metrics agent | `go run ./cmd/agent` | `127.0.0.1:8081` |
| 2: demo application | `go run ./cmd/demoapp` | `127.0.0.1:8000` |
| 3: controller | `go run ./cmd/controller` | `127.0.0.1:8080` |

Wait about five seconds for the first controller checks. In another terminal, run:

```sh
curl http://127.0.0.1:8081/metrics
curl http://127.0.0.1:8000/health
curl http://127.0.0.1:8080/api/status
curl 'http://127.0.0.1:8080/api/metrics?limit=10'
curl 'http://127.0.0.1:8080/api/traffic?limit=10'
curl 'http://127.0.0.1:8080/api/events?limit=10'
```

The status response should contain `"metrics_available":true` and `"app_health":{"status":"healthy",...}`. `"state":"UNCONFIGURED"` and `"hardware_ready":false` are expected until the Pi and wake/routing hardware are connected. Sample times are UTC; network byte values are cumulative counters. `/api/metrics` returns newest samples first, and `/api/metrics/latest` returns the newest one. The dashboard shows current readings, recent charts, health, events, and Traffic Lab controls: run `npm ci` once in `web/`, then `npm run dev` there and open `http://127.0.0.1:3000`.

`/api/traffic` records app request counts, HTTP 4xx/5xx counts, request rate, error percentage, average response time, and an interval p95 latency estimate from bounded histogram buckets. The first sample has no rates because it needs a previous reading. Generate requests after the controller has run for five seconds, then read `/api/traffic` again. The demo app's `/health` and `/stats` probes are excluded from traffic counts. `/api/events` lists changes in app health, metrics availability, and traffic-stats availability; it is stored in the same SQLite file. To exercise traffic, run this in the fourth terminal:

```sh
for i in {1..20}; do curl -fsS 'http://127.0.0.1:8000/workload?duration_ms=500' >/dev/null || break; printf 'request %s/20 complete\n' "$i"; done
curl -i 'http://127.0.0.1:8000/workload?duration_ms=0'
curl http://127.0.0.1:8000/stats
curl 'http://127.0.0.1:8080/api/traffic?limit=5'
curl 'http://127.0.0.1:8080/api/events?limit=10'
```

The invalid `duration_ms=0` request returns HTTP 400 and increments the app error count. Requests per second and average latency use differences between consecutive five-second snapshots, so wait for the next controller poll if the newest sample still shows an earlier interval.

## Run the Python traffic model

The [model guide](model/README.md) documents the real Azure Functions data, paper and author implementation that inspired the method, training command, held-out results, and limitations. The trained artifact is in `model/artifacts/`. Install its pinned Python dependencies, then start the backend:

The active v2 artifact is about 193 KB and needs NumPy, SciPy, scikit-learn, and joblib for scoring. Its latest retraining produced small pooled precision and recall gains over the previous v2 on inspected Azure groups; an independent workload is still needed to confirm them. Raspberry Pi deployment requires a 64-bit OS with compatible Python packages and a benchmark on the actual board; current timings are from this laptop.

```sh
python3 -m pip install -r model/requirements.txt
./start-backend.sh
```

The launcher starts the Python scoring service when `model/artifacts/serverless_model.joblib` exists. The Go controller forwards `/api/prediction` to that service. It needs **60 complete minutes** of request history before returning a score. Until then, the dashboard shows the reason that prediction is unavailable. Once history exists, check:

```sh
curl http://127.0.0.1:8080/api/prediction
```

The score means the chance of a **traffic surge** in the next two minutes, with a predicted request rate and upper estimate. It does not mean this laptop will overload. Set `MODEL_CAPACITY_RPM` to measured one-server capacity to see a **shadow** `would_wake` or `stay_primary` decision. The model is trained on real Azure request traces, but those traces contain no measured capacity or benefit from adding this project's second server. **Predictions stay shadow-only by default.** The [Pi setup guide](docs/windows-pi-setup.md#8-ai-behavior) documents the separate switches required after physical validation. The CPU activation path works without model availability when hardware control is explicitly enabled.

Restart `./start-backend.sh` after changing Go code. The SQLite file defaults to `data/monitor.sqlite3` and is ignored by Git; saved samples survive a controller restart. If `curl` says “Couldn't connect to server,” check that the startup terminal is still running. If the app is stopped separately, `app_health.status` becomes `unhealthy` while metrics can remain available; if the agent is stopped separately, `metrics_available` becomes false while the app can remain healthy. Historical samples stay in the database in both cases.

For different hosts, set `AGENT_ADDR` and `DEMOAPP_ADDR` to private LAN bind addresses on the laptops, then set `AGENT_URL` and `APP_URL` on the controller to those hosts' URLs. `CONTROLLER_ADDR` changes the controller bind address; `SQLITE_PATH` changes the database path; `METRICS_DISK_PATH` changes the measured disk (default `/`). `MODEL_URL` points the controller at the private Python service (default `http://127.0.0.1:8090`), `MODEL_ARTIFACT` selects the local joblib file, and `MODEL_CAPACITY_RPM` enables the shadow capacity decision. `config/example.yaml` records planned network values but is not loaded by the running services yet. Do not expose these prototype endpoints to the public internet.

## Current service boundaries

| Service | Available endpoints | Responsibility |
| --- | --- | --- |
| Go controller | `GET /health`, `GET /api/status`, `GET /api/metrics`, `GET /api/metrics/latest`, `GET /api/traffic`, `GET /api/events`, `GET /api/prediction` | Polls one agent, app health, and app stats independently; stores samples and events; forwards prediction requests to Python |
| Python model | `GET /health`, `GET /prediction` | Builds minute request history and scores the trained traffic model |
| Go agent | `GET /health`, `GET /identity`, `GET /metrics` | Reads CPU, memory, disk, network counters, uptime, and optional temperature |
| Go demo app | `GET /health`, `GET /`, `GET /workload`, `GET /stats` | Stateless app with bounded CPU work and cumulative request counters |
| Next.js dashboard | `GET /` | Reads the private Go API on the server and refreshes status, trends, and events every five seconds |

The controller owns Wake-on-LAN, scaling decisions, and HAProxy changes when `SCALING_ENABLED=true` and hardware settings are configured. `GET /api/servers` and `/api/scaling` expose both servers and activation state; history endpoints accept `server=primary|standby`. Authenticated `POST /api/standby/activate`, `/retry`, and `/reset` handle operator actions. Next.js remains the frontend and calls the Pi API server-side. An unavailable controller appears as unavailable on the dashboard rather than as a healthy machine.

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
3. Verify both server histories and exercise the implemented activation state machine with the actual hardware settings.
4. Exercise dashboard controls, failure recovery, and the event timeline; record a repeatable demo.

Copy `config/example.yaml` to `config/local.yaml` when device addresses and the standby MAC are known. Keep real settings and secrets out of version control.
