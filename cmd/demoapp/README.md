# Demo application

This stateless Go demo app exposes `GET /health`, `GET /`, bounded `GET /workload`, and `GET /stats`. The root response identifies the serving laptop. `/stats` returns cumulative client request count, HTTP 4xx/5xx count, and total response time since app start; health and stats probes are excluded. Run `go run ./cmd/demoapp` from the repository root; it listens on `127.0.0.1:8000` by default. Set `DEMOAPP_ADDR` to change the bind address or `SERVER_NAME` to change the displayed name. The controller checks `/health` and `/stats` independently every five seconds.

## Simulated users

Open `/users` (or `/`) for the serving server identity, `active_users` and
`total_users_served`. Each in-flight `/workload` request represents one simulated
user visit; completed visits are cumulative since app startup, not unique people.
Health, stats and user-count reads do not add workload requests to `/stats`.
`/stats` also includes live `users` counts so the Pi can expose them through
`/api/servers`. The dashboard's Traffic Lab reads these counts from the Pi.
