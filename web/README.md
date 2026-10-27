# Dashboard

The frontend is Next.js with TypeScript. Its Node.js process serves the dashboard and reads the private Go API. Go owns monitoring and scaling; there is no separate Node backend.

Run `./start-backend.sh` from the repository root. Then run `npm ci` once and `npm run dev` in this directory and open `http://127.0.0.1:3000`. Set `CONTROLLER_API_URL` in `web/.env.local` to change the private Go API address; the default is `http://127.0.0.1:8080`.

The dashboard shows both server identities, CPU/memory, per-server request rates and CPU history, primary trends, health, events, model forecasts, and actual scaling/proxy state. It refreshes every five seconds. If a source stops, its current values become unavailable while historical charts remain visible. The prediction panel reads the Python service through the Go API and needs 60 complete minutes of primary request history. CPU activation works independently when enabled; model activation stays shadow-only until explicit hardware validation switches are configured. [Traffic Lab](../demo/traffic-lab/README.md) supports presets and bounded custom loads. Operator actions use the private `CONTROL_TOKEN` server-side.

The Next.js server forwards preset start and stop commands to Traffic Lab at `TRAFFIC_LAB_URL` (default `http://127.0.0.1:8091`). The load generator itself stays in Go; Node.js only handles dashboard requests. Use the default loopback addresses for the single-machine prototype.

## Presentation from Laptop 1 with the Pi API

The target deployment runs Next.js and Traffic Lab on Laptop 1, with monitoring, inference, storage, and HAProxy on the Raspberry Pi. Open `http://127.0.0.1:3000` on Laptop 1. In `web/.env.local`, set `CONTROLLER_API_URL=http://<pi-LAN-address>:8080`, replacing the placeholder with the actual Pi address, then restart Next.js. The Pi API must listen on its LAN interface and permit the presentation laptop's connection. The existing server-side requests support this split; browser CORS configuration is unnecessary because Next.js makes the Pi calls.

Keep `TRAFFIC_LAB_URL=http://127.0.0.1:8091` when the generator runs on Laptop 1. Set `TRAFFIC_LAB_TARGET=http://<pi-LAN-address>:8088` in the Go generator's environment before starting it so traffic reaches the Pi proxy. Closing the dashboard leaves Pi monitoring running. The all-in-one backend launcher is for local development; the supplied `start-pi.sh` and `start-windows.ps1` launch the distributed setup. See [Windows/Pi startup and tests](../docs/windows-pi-setup.md).

The activation view shows the configured CPU warning/activation thresholds, full hold progress, model availability, wake/health/routing state, and authenticated activate/retry/drain-reset controls. Defaults are 60%, 70%, and 15 seconds. Hardware actions require configured standby and proxy settings plus `SCALING_ENABLED=true` on the Pi. Set matching `CONTROL_TOKEN` values on Pi and dashboard; keep this variable private, without a `NEXT_PUBLIC_` prefix.
