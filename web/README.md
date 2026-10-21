# Dashboard

The frontend is Next.js with TypeScript. Its Node.js process serves the dashboard and reads the private Go API. Go owns monitoring and scaling; there is no separate Node backend.

Run `./start-backend.sh` from the repository root. Then run `npm ci` once and `npm run dev` in this directory and open `http://127.0.0.1:3000`. Set `CONTROLLER_API_URL` in `web/.env.local` to change the private Go API address; the default is `http://127.0.0.1:8080`.

The current dashboard shows controller connectivity, current CPU and memory, request rate, average response time, recent charts, app and agent health, saved controller events, and an overload prediction preview. It refreshes every five seconds. If a source stops, its current values become unavailable while historical charts remain visible. The prediction panel needs a model loaded by the Go controller and fresh metrics and traffic; it shows why scoring is unavailable otherwise. The included training data is synthetic and the score does not activate hardware. See the [AI preview runbook](../README.md#try-the-ai-preview). Proxy targets and scaling actions are planned.
