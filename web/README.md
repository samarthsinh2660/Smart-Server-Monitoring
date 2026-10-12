# Dashboard

The frontend is Next.js with TypeScript. Its Node.js process serves the dashboard and reads the private Go API. Go owns monitoring and scaling; there is no separate Node backend.

Run `go run ./cmd/controller` from the repository root. Then run `npm install` and `npm run dev` in this directory and open `http://127.0.0.1:3000`. Set `CONTROLLER_API_URL` in `web/.env.local` to change the private Go API address; the default is `http://127.0.0.1:8080`.

The dashboard will show laptop health and metrics, active proxy targets, controller state, and a time ordered event log. Manual actions will call the controller API.
