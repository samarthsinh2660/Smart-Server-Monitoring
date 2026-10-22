# Traffic Lab for the faculty demo

This small Go service generates real HTTP requests against the existing Go demo app. It lives in a separate folder from the monitored application. The Next.js dashboard shows controls for four fixed scenarios and continues to display the resulting app traffic, latency, CPU, and event history.

From the repository root, run `./start-backend.sh`, then start the dashboard with `cd web && npm run dev`. Open `http://127.0.0.1:3000` and use **Traffic Lab** below the charts. The backend launcher starts this service on `127.0.0.1:8091` and the demo app on `127.0.0.1:8000`. The dashboard calls its same-origin Next.js route, which forwards only preset start and stop commands to the local Go service.

| Scenario | Traffic | Duration | App work per request |
| --- | ---: | ---: | ---: |
| Steady | 2 requests/s | 2 min | 100 ms |
| Busy | 6 requests/s | 2 min | 200 ms |
| Burst | 12 requests/s | 45 sec | 350 ms |
| Ramp | 2 to 12 requests/s | 90 sec | 200 ms |

One scenario runs at a time. Stop cancels in-flight requests. The generator permits at most eight concurrent requests and records requests sent, completed, failed, cancelled, and skipped when concurrency is full. The scenario ends automatically; it does not wake a standby or change routing.

For separate debugging, run `go run ./demo/traffic-lab` from the repository root after starting `go run ./cmd/demoapp`. `TRAFFIC_LAB_ADDR` changes the listener (default `127.0.0.1:8091`), `TRAFFIC_LAB_TARGET` changes the fixed HTTP origin receiving load (default `http://127.0.0.1:8000`), and `TRAFFIC_LAB_URL` tells the Next.js server where to reach the generator. Keep the generator and dashboard on a trusted development network; the prototype has no user accounts or public access controls.

The controller polls app statistics every five seconds, so the dashboard may take one or two refreshes to show a new rate. The AI card needs **60 complete minutes** of traffic history before it can score. Run the backend beforehand if the forecast must be shown during the presentation. Short presets still demonstrate live traffic and system metrics immediately.
