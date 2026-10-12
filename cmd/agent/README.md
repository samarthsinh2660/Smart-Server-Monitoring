# Laptop metrics agent

The Linux agent exposes `GET /metrics` with a timestamped CPU, memory, disk, network counter, uptime, and optional temperature sample. It also exposes `GET /health` and `GET /identity`. Run `go run ./cmd/agent` from the repository root. Set `AGENT_ADDR` to a private LAN bind address when the controller runs on another machine.
