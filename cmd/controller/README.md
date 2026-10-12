# Pi controller

The Go controller polls one agent every five seconds and saves its samples in SQLite. Run `go run ./cmd/controller` from the repository root; set `AGENT_URL` and `SQLITE_PATH` if needed. `GET /api/status` reports sample availability, while `GET /api/metrics` and `GET /api/metrics/latest` read history. Application health checks, scaling decisions, Wake-on-LAN, and proxy changes are future work.
