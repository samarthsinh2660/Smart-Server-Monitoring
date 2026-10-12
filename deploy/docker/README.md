# Laptop application deployment

`Dockerfile` builds the stateless Go demo app. Run the same `compose.yaml` on each laptop, setting a different `SERVER_NAME` on each one. [Compose's `unless-stopped` policy](https://docs.docker.com/reference/compose-file/services/#restart) restarts the app when Docker starts after a normal boot, provided the container was not manually stopped.

From the repository root on each laptop:

```sh
SERVER_NAME=primary docker compose -f deploy/docker/compose.yaml up -d --build
```

Use `SERVER_NAME=standby` on Laptop 2. `APP_PORT` changes the published host port from the default `8000`; `APP_BIND_IP` changes the bind address. The app inside the container always listens on port `8000`. Allow the Pi to reach that host port when the hardware network is ready.

The app exposes `GET /health`, `GET /`, and `GET /workload?duration_ms=250`. Workload duration is limited to 1–2000 ms per request. Concurrent workload requests can produce sustained CPU load for the later scaling demonstration.
