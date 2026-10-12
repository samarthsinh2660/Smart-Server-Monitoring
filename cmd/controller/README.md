# Pi controller

The Go controller polls one agent and one demo app every five seconds. It saves metric samples in SQLite, while `GET /api/status` reports metrics availability and application health independently. Run `go run ./cmd/controller` from the repository root; set `AGENT_URL`, `APP_URL`, and `SQLITE_PATH` if needed. `GET /api/metrics` and `GET /api/metrics/latest` read history. Scaling decisions, Wake-on-LAN, and proxy changes are future work.
