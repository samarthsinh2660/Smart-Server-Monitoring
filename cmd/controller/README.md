# Pi controller

The Go controller polls one agent and one demo app every five seconds. It saves laptop metrics, app traffic snapshots, and availability-change events in SQLite. `GET /api/status` reports metrics, traffic stats, and application health independently; `GET /api/metrics`, `GET /api/metrics/latest`, `GET /api/traffic`, and `GET /api/events` read history. Run `go run ./cmd/controller` from the repository root; set `AGENT_URL`, `APP_URL`, and `SQLITE_PATH` if needed. Scaling decisions, Wake-on-LAN, and proxy changes are future work.
