# Dashboard

The frontend is Next.js with TypeScript. Its Node.js process serves the dashboard and reads the private Go API. Go owns monitoring and scaling; there is no separate Node backend.

Run the agent, demo app, and controller as described in the [repository README](../README.md). Then run `npm ci` and `npm run dev` in this directory and open `http://127.0.0.1:3000`. Set `CONTROLLER_API_URL` in `web/.env.local` to change the private Go API address; the default is `http://127.0.0.1:8080`.

The current dashboard shows controller connectivity, hardware readiness, laptop metrics availability, and demo app health. Charts, proxy targets, an event timeline, and manual actions are planned.
