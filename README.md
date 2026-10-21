# Smart Server Monitor

A Raspberry Pi controlled prototype that monitors a primary laptop, wakes a standby laptop with Wake-on-LAN, checks application health, and changes traffic routing. See [the project idea](docs/idea.md) for the full design.

## Tech stack

- **Backend and device agents:** Go. The controller/API, laptop metrics agent, and demo application are planned in Go.
- **Frontend:** Next.js with TypeScript. Node.js runs the Next.js dashboard; Go remains the only monitoring and scaling backend.
- **Routing:** HAProxy on the Pi.
- **Storage:** SQLite for timestamped metrics and controller events.
- **AI preview:** Small logistic regression model trained and evaluated in Go; inference runs in the controller.
- **Deployment:** Docker Compose for the laptop app; systemd for controller and agent services.
- **Network control:** Magic-packet wake over the local network. The current laptop reports Wi-Fi WoWLAN capability; an actual wake trial is still pending.

## Repository layout

| Path | Runs on | Responsibility |
| --- | --- | --- |
| `cmd/controller/` | Raspberry Pi | Controller and HTTP API entry point |
| `cmd/agent/` | Both laptops | Metrics agent entry point |
| `cmd/demoapp/` | Both laptops | Stateless application entry point |
| `cmd/trainmodel/` | Development laptop | Train and evaluate a small model from labelled feature rows |
| `cmd/recordtrace/` | Development laptop | Capture real feature runs and label future CPU overload |
| `internal/controller/` | Raspberry Pi | Metrics polling and independent app health checks; later, scaling decisions |
| `internal/metrics/` | Pi and laptops | Metrics collection and transport |
| `internal/config/` | Pi and laptops | Configuration loading |
| `internal/proxy/` | Raspberry Pi | HAProxy target management |
| `internal/store/` | Raspberry Pi | SQLite metrics, traffic, and controller events |
| `internal/prediction/` | Development laptop and Pi | Feature extraction, model format, training, and inference |
| `web/` | Raspberry Pi | Next.js status dashboard |
| `start-backend.sh` | Development laptop | Build and start all three Go services together |
| `config/example.yaml` | Raspberry Pi | Example topology and policy values |
| `deploy/` | Pi and laptops | HAProxy, Docker, and systemd setup |

The Go agent can read live Linux laptop metrics. The Go controller polls one agent every five seconds and stores samples locally. Scaling and physical hardware integration remain future phases.

## Run the laptop prototype

You need a Linux laptop, Go 1.22 or newer, and a C compiler for the controller's SQLite driver. From the repository root, start the agent, demo app, and controller with one command:

```sh
./start-backend.sh
```

Leave that terminal open. Ctrl+C stops all three services. The script builds the Go binaries into ignored `bin/` files before starting them. Stop any older copies of these services first, or their ports will already be in use.

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

The status response should contain `"metrics_available":true` and `"app_health":{"status":"healthy",...}`. `"state":"UNCONFIGURED"` and `"hardware_ready":false` are expected until the Pi and wake/routing hardware are connected. Sample times are UTC; network byte values are cumulative counters. `/api/metrics` returns newest samples first, and `/api/metrics/latest` returns the newest one. The dashboard shows current readings, recent charts, health, and events: run `npm ci` once in `web/`, then `npm run dev` there and open `http://127.0.0.1:3000`.

`/api/traffic` records app request counts, HTTP 4xx/5xx counts, request rate, error percentage, average response time, and an interval p95 latency estimate from bounded histogram buckets. The first sample has no rates because it needs a previous reading. Generate requests after the controller has run for five seconds, then read `/api/traffic` again. The demo app's `/health` and `/stats` probes are excluded from traffic counts. `/api/events` lists changes in app health, metrics availability, and traffic-stats availability; it is stored in the same SQLite file. To exercise traffic, run this in the fourth terminal:

```sh
for i in {1..20}; do curl -fsS 'http://127.0.0.1:8000/workload?duration_ms=500' >/dev/null || break; printf 'request %s/20 complete\n' "$i"; done
curl -i 'http://127.0.0.1:8000/workload?duration_ms=0'
curl http://127.0.0.1:8000/stats
curl 'http://127.0.0.1:8080/api/traffic?limit=5'
curl 'http://127.0.0.1:8080/api/events?limit=10'
```

The invalid `duration_ms=0` request returns HTTP 400 and increments the app error count. Requests per second and average latency use differences between consecutive five-second snapshots, so wait for the next controller poll if the newest sample still shows an earlier interval.

## Try the AI preview

The checked-in CSV is **synthetic demonstration data**. Its model proves the training and inference path, but its percentages are not a reliable forecast of this laptop. Train it once from the repository root:

```sh
go run ./cmd/trainmodel -input examples/overload-training.csv -holdout-run burst-b -version demo-v2 -output data/overload-model.json
```

The command reports accuracy and positive recall on the held-out `burst-b` run. The generated JSON weights are local runtime data and are ignored by Git. Restart the Go services with the model path set:

```sh
MODEL_PATH=data/overload-model.json ./start-backend.sh
```

Start the dashboard separately from `web/` with `npm run dev`. The model needs at least six recent metric samples, six traffic samples, and a completed app request within the last 30 seconds. With the services running, generate about 40 seconds of steady traffic in another terminal:

```sh
for i in {1..80}; do curl -fsS 'http://127.0.0.1:8000/workload?duration_ms=500' >/dev/null || break; done
curl http://127.0.0.1:8080/api/prediction
```

The endpoint returns `available:true`, a `risk_score` from 0 to 1, model version, source, timestamp, and input features when it has fresh data. Otherwise it returns `available:false` with a reason. The dashboard shows the same state. The model uses interval p95 latency and its trend; p95 is a histogram bucket estimate, so its resolution depends on the bucket boundaries. The 120-second horizon in the example is provisional until WoL-to-health time is measured. With recorded weights, three consecutive scores at or above 0.8 write a clearly labelled **shadow trigger** to `/api/events`. Synthetic weights cannot trigger it. **No score wakes the standby or alters routing yet.**

For real training data, use the model-independent `GET /api/features` endpoint and `go run ./cmd/recordtrace -run-id <unique-name> -duration 5m -horizon 2m` while a matching workload runs. The recorder appends labelled rows to ignored `data/recorded-traces.csv`. Read the [model decision and recorded-data workflow](docs/ai-model.md) before using recorded weights; the development model must not drive physical activation.

Restart `./start-backend.sh` after changing Go code. The SQLite file defaults to `data/monitor.sqlite3` and is ignored by Git; saved samples survive a controller restart. If `curl` says “Couldn't connect to server,” check that the startup terminal is still running. If the app is stopped separately, `app_health.status` becomes `unhealthy` while metrics can remain available; if the agent is stopped separately, `metrics_available` becomes false while the app can remain healthy. Historical samples stay in the database in both cases.

For different hosts, set `AGENT_ADDR` and `DEMOAPP_ADDR` to private LAN bind addresses on the laptops, then set `AGENT_URL` and `APP_URL` on the controller to those hosts' URLs. `CONTROLLER_ADDR` changes the controller bind address; `SQLITE_PATH` changes the database path; `METRICS_DISK_PATH` changes the measured disk (default `/`). `config/example.yaml` records planned network values but is not loaded by the running services yet. Do not expose these prototype endpoints to the public internet.

## Current service boundaries

| Service | Available endpoints | Responsibility |
| --- | --- | --- |
| Go controller | `GET /health`, `GET /api/status`, `GET /api/metrics`, `GET /api/metrics/latest`, `GET /api/traffic`, `GET /api/events`, `GET /api/features`, `GET /api/prediction` | Polls one agent, app health, and app stats independently; stores samples and events; scores fresh inputs when a model is configured |
| Go agent | `GET /health`, `GET /identity`, `GET /metrics` | Reads CPU, memory, disk, network counters, uptime, and optional temperature |
| Go demo app | `GET /health`, `GET /`, `GET /workload`, `GET /stats` | Stateless app with bounded CPU work and cumulative request counters |
| Next.js dashboard | `GET /` | Reads the private Go API on the server and refreshes status, trends, and events every five seconds |

The controller will later own Wake-on-LAN, scaling decisions, and HAProxy changes. Next.js is only the frontend. An unavailable controller appears as unavailable on the dashboard rather than as a healthy machine.

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
