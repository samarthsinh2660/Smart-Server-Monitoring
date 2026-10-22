# Dashboard

The frontend is Next.js with TypeScript. Its Node.js process serves the dashboard and reads the private Go API. Go owns monitoring and scaling; there is no separate Node backend.

Run `./start-backend.sh` from the repository root. Then run `npm ci` once and `npm run dev` in this directory and open `http://127.0.0.1:3000`. Set `CONTROLLER_API_URL` in `web/.env.local` to change the private Go API address; the default is `http://127.0.0.1:8080`.

The current dashboard shows controller connectivity, current CPU and memory, request rate, average response time, recent charts, app and agent health, saved controller events, a traffic-surge forecast preview, and [Traffic Lab](../demo/traffic-lab/README.md) controls for generating demo load. It refreshes every five seconds. If a source stops, its current values become unavailable while historical charts remain visible. The prediction panel reads the Python model service through the Go API and needs 60 complete minutes of request history. The model was trained on real Azure Functions trace data, but its score does not activate hardware. See the [Python model runbook](../README.md#run-the-python-traffic-model). Proxy targets and scaling actions are planned.

The Next.js server forwards preset start and stop commands to Traffic Lab at `TRAFFIC_LAB_URL` (default `http://127.0.0.1:8091`). The load generator itself stays in Go; Node.js only handles dashboard requests. Use the default loopback addresses for the local presentation.
