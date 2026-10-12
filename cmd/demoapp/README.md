# Demo application

This stateless Go demo app exposes `GET /health`, `GET /`, and bounded `GET /workload`. The root response identifies the serving laptop. Run `go run ./cmd/demoapp` from the repository root; it listens on `127.0.0.1:8000` by default. Set `DEMOAPP_ADDR` to change the bind address or `SERVER_NAME` to change the displayed name. The controller checks `/health` every five seconds, independently of the metrics agent.
