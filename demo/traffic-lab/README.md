# Traffic Lab for the faculty demo

This small Go service generates real HTTP requests against the existing Go demo app. It lives in a separate folder from the monitored application. The Next.js dashboard lets the presenter switch between traffic rates while showing the resulting app traffic, latency, CPU, and event history.

From the repository root, run `./start-backend.sh`, then start the dashboard with `cd web && npm run dev`. Open `http://127.0.0.1:3000` and use **Traffic Lab** below the charts. The backend launcher starts this service on `127.0.0.1:8091` and the demo app on `127.0.0.1:8000`. The dashboard calls its same-origin Next.js route, which forwards only preset start and stop commands to the local Go service.

| Mode | Rate choices | Duration | App work per request | Concurrent request limit |
| --- | --- | ---: | ---: | ---: |
| Requests/minute | 200, 500, 1,000, 2,000; ramp 200 → 2,000 | 2 min | 150–250 ms; 200 ms during ramp | 32 |
| Requests/second | 200, 500, 1,000, 2,000; ramp 200 → 2,000 | 90 sec | 2 ms | 256 |

The dashboard also accepts custom rate (1–2,000 in the selected unit), concurrency (1–256), CPU work per request (1–2,000 ms), and duration (1–3,600 seconds). Go validates each bound. Apply a custom load or another preset to replace the current run. The app independently defaults to 32 concurrent CPU workloads and returns counted HTTP 429 errors above its limit. The generator reports attempts, successes, failures, skipped work, and achieved run-average RPS; the controller charts show measured app intervals.

One scenario runs at a time. Choosing another rate cancels the current scenario and starts the new one, so the presenter can increase or decrease traffic without restarting services. Stop cancels in-flight requests. The generator records requests sent, completed, failed, cancelled, and skipped when concurrency is full. A scenario ends automatically; it does not wake a standby or change routing. At 2,000 requests/second the **requested** rate may exceed what this laptop can deliver; compare the dashboard's measured request-rate chart and the generator counters before reporting throughput.

For separate debugging, run `go run ./demo/traffic-lab` from the repository root after starting `go run ./cmd/demoapp`. `TRAFFIC_LAB_ADDR` changes the listener (default `127.0.0.1:8091`), `TRAFFIC_LAB_TARGET` changes the fixed HTTP origin receiving load (default `http://127.0.0.1:8000`), and `TRAFFIC_LAB_URL` tells the Next.js server where to reach the generator. Keep the generator and dashboard on a trusted development network; the prototype has no user accounts or public access controls.

The controller polls app statistics every five seconds, so the dashboard may take one or two refreshes to show a new rate. The AI card needs **60 complete minutes** of traffic history before it can score. Run the backend beforehand if the forecast must be shown during the presentation. Short scenarios still demonstrate live traffic and system metrics immediately.

For the distributed Windows deployment, the generator runs on Laptop 1 and `TRAFFIC_LAB_TARGET` points to the Pi proxy on port 8088. Monitoring and hardware control stay on the Pi; an enabled controller may activate the standby as generated load crosses its policy. Use [the Windows/Pi setup guide](../../docs/windows-pi-setup.md) rather than the local all-in-one launcher.

The model uses requests **per minute** internally. At 2,000 requests/second, that is a target of 120,000 requests/minute, beyond the range used for the lighter CPU scenarios. Its `would_wake` output is shadow-only by default; explicitly validated AI can be enabled separately from CPU activation. Set `MODEL_CAPACITY_RPM` only after measuring the primary's sustainable rate for the same request cost: the CPU-heavy and lightweight modes have different capacities. More RAM alone does not determine either rate.
