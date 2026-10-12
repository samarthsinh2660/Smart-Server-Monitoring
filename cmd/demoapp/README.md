# Demo application

This stateless Go demo app exposes `GET /health`, `GET /`, bounded `GET /workload`, and `GET /stats`. The root response identifies the serving laptop. `/stats` returns cumulative client request count, HTTP 4xx/5xx count, and total response time since app start; health and stats probes are excluded. Run `go run ./cmd/demoapp` from the repository root; it listens on `127.0.0.1:8000` by default. Set `DEMOAPP_ADDR` to change the bind address or `SERVER_NAME` to change the displayed name. The controller checks `/health` and `/stats` independently every five seconds.
