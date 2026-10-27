# Handoff to Codex on Windows

Prepared on 2026-10-27. Start here when continuing this project after the user's dual-boot laptop switches from Linux to Windows. The implementation is written and cross-built; the next task is native Windows verification, fixes based on actual results, and integration with the Raspberry Pi and second Windows laptop.

## User requirements to preserve

- Raspberry Pi **4 Model B**, running a 64-bit Linux OS, owns monitoring/control, SQLite, Python inference, and HAProxy.
- Both target application laptops run **Windows**. Run the Go app and agent natively; the user does not want Linux or WSL as a prerequisite on the laptops.
- Laptop 1 is the primary app server and presentation host. Its Next.js dashboard reads the **Pi API**. Its Go Traffic Lab generates requests through the **Pi HAProxy** entry point.
- Laptop 2 is the standby. The Pi sends a Wi-Fi magic packet, waits for its app and proxy health, then routes to it.
- All devices communicate over the **same local Wi-Fi network**. Wireless operation is a constraint. A mobile hotspot is acceptable only if the actual communication and wake trials pass. Ethernet wake alone does not satisfy the requirement.
- CPU warning is **60%**. CPU activation is **at least 70% for a full 15 seconds** of distinct fresh samples. This is an activation rule, not a CPU cap or a crash-prevention guarantee.
- Validated AI can request earlier activation; missing/unvalidated AI must not stop the CPU or app-failure paths. Preserve the selected trained model; do not start another training campaign as part of Windows deployment.
- Preserve the existing repository structure and detailed documentation. Go is the application/control backend, Next.js is the frontend, and Python is confined to model work and Pi inference.

## Read these files next

1. [Windows and Pi setup guide](windows-pi-setup.md): complete commands, firewall, startup, physical checks, AI switches.
2. [Deployment design](windows-wireless-deployment.md): device boundaries, API/data flow, policy and acceptance criteria.
3. [Project idea](idea.md) and [roadmap](README.md): broader context and remaining physical phase gates.
4. [Implementation plan and verification record](superpowers/plans/2026-10-27-windows-pi-implementation.md).
5. [Model runbook](../model/README.md): artifact, real training data, evaluation results, limitations. A traffic forecast is not a CPU prediction.

## Implementation map

| Location | Responsibility |
| --- | --- |
| `internal/metrics/collect_windows.go` | Native GetSystemTimes, GlobalMemoryStatusEx, GetDiskFreeSpaceExW, GetTickCount64, GetIfEntry2 collection; compile-time ABI guards |
| `internal/metrics/collect_windows_test.go` | Native Windows layout and measurement checks |
| `cmd/agent/` | `/metrics`, `/identity`, `/health`; Windows system drive is supported |
| `cmd/demoapp/` | `/health`, `/stats`, serving identity, bounded real CPU workload; default workload concurrency limit 32, excess returns counted HTTP 429 |
| `internal/store/` | Additive `server_id` migrations, separate histories, atomic control-state/transition persistence |
| `internal/controller/server.go`, `poller.go`, `traffic.go`, `health.go` | Independent primary/standby telemetry and app-health polling |
| `internal/controller/scaling.go` | CPU hold, failover, activation lifecycle, restart reconciliation, timeout, guarded drain/reset |
| `internal/controller/wake.go` | Validated 102-byte magic packet and UDP sender |
| `internal/proxy/runtime.go` | Local HAProxy Unix socket, CSV status, fixed primary/standby target state changes |
| `internal/config/runtime.go`, `config/pi.env.example` | Validated environment configuration; YAML is not loaded |
| `internal/controller/prediction.go`, `model/serve.py` | Cached predictions, freshness checks, explicit operational AI gates |
| `demo/traffic-lab/` | Presets and custom RPM/RPS, concurrency, CPU work, duration, cancellation, achieved throughput |
| `web/app/` | Primary/standby cards/history, scaling status/hold progress, authenticated operator actions, custom load controls |
| `start-windows.ps1` | Native Windows app/agent; optional primary presentation dashboard and generator |
| `install-windows-startup.ps1` | Standby scheduled startup as SYSTEM and Pi-scoped private-network firewall rules |
| `start-pi.sh`, `deploy/systemd/smartserver.service` | Pi controller plus optional inference; failed inference leaves the controller running |
| `build-windows.sh` | Builds Windows amd64 executables and native collector test executable from Linux |

## Current validation: keep these distinctions

Passed on Linux during implementation:

- `GOCACHE=/tmp/iot-go-build go test -race ./...`.
- `python3 -m unittest model.test_model` — eight model regression tests.
- `python3 -m unittest deploy.test_pi_launcher` — model missing/failure does not stop controller; cleanup stops owned children. Service executables are stubbed for this process-isolation test.
- Windows amd64 app, agent, generator, and metrics-test **cross-builds**.
- Windows-targeted `go vet` for app, agent, metrics, and Traffic Lab.
- Next.js TypeScript and `npm run build -- --webpack` production build.
- Bash launcher syntax and Git whitespace checks.

Not yet verified on the actual devices:

- Windows PowerShell parsing/execution, native collector values and HTTP endpoints.
- Windows firewall, scheduled startup without login, and resume after sleep.
- Pi ARM64 Python dependency installation, inference memory/latency, controller build and socket permissions.
- Pi-to-Windows Wi-Fi wake, standby wake-to-app-health time, actual two-host HAProxy routing and service improvement.

Do not describe a cross-build or injected-device test as a successful hardware trial. Unsupported Windows temperature is omitted, not reported as zero. Missing metrics stay unavailable.

## Transfer and repository state

Continue from a source checkout containing the latest committed implementation. The optional transfer ZIP was removed at the user's request. `bin/` is ignored by Git; a source clone does not include `.exe` files. Install Go >=1.22 on Windows and let the Windows launcher build the binaries. Model artifacts, configuration examples and documentation are already in the source repository; large training data is unnecessary for deployment.

Before reboot, ensure Windows can access the source checkout, or push the local commits and clone/pull them on Windows using the user's SSH transport. If reusing a working copy, preserve its `.git` and unrelated changes. Existing prebuilt binaries may be copied manually to a Windows-readable drive as a convenience; no archive is required. Do not transfer local secrets, databases, Linux Node dependencies or training data as part of setup.

The implementation and this handoff are committed on `main` under **`Add Windows Pi scaling demo`**. The base before this work was `bd71a4a` (`Add dashboard traffic demo`). Earlier Linux Codex turns rejected Git staging with:

```text
fatal: Unable to create '.../.git/index.lock': Read-only file system
```

Staging and committing succeeded during the handoff turn; the earlier restriction no longer blocked this commit. Inspect `git status` and `git log` when resuming. This turn created a local commit and did not push it, so do not assume the implementation reached the remote. The observed remote is HTTPS, while the user requested SSH for Git transport. Preserve existing working authentication and use this SSH URL when configuring transport:

```text
ssh://git@github.com/samarthsinh2660/Smart-Server-Monitoring.git
```

The requested short commit message is:

```text
Add Windows Pi scaling demo
```

Commit source and documentation after reviewing the diff. Do not commit local credentials, real network settings, generated binaries, databases, or training data. Do not force-push or discard unrelated work.

## First work on Windows

1. Inspect `git status`, locate the source checkout, install Go >=1.22 if building there, and read this handoff and the setup guide. Do not assume this Linux absolute path exists on Windows.
2. Parse `start-windows.ps1` and `install-windows-startup.ps1` with PowerShell's parser before running them; fix any native platform issue found. These scripts have not been executed under Windows in this session.
3. Run the collector test first, from the project root:

   ```powershell
   go test ./internal/metrics
   ```

4. Start the primary app and agent without presentation while checking the native endpoints:

   ```powershell
   powershell -NoProfile -ExecutionPolicy Bypass -File .\start-windows.ps1 -Role primary
   ```

   In another terminal:

   ```powershell
   curl.exe http://127.0.0.1:8000/health
   curl.exe http://127.0.0.1:8000/
   curl.exe http://127.0.0.1:8081/metrics
   ```

   Compare several CPU/RAM samples with Windows tools, allowing for different sampling intervals. Fix measured platform errors rather than returning synthetic values. Logs are in `.logs/`.

5. If Go is installed, run focused native tests:

   ```powershell
   go test ./internal/metrics ./internal/traffic ./demo/traffic-lab
   ```

   Full `go test ./...` includes the Pi controller's CGO SQLite tests. Do not install a Windows C toolchain solely to run Pi-only packages; run the full suite on Linux/Pi with its C compiler. Windows app/agent binaries need no CGO or Python.

6. Install Node.js >=20.9 for Laptop 1. Run `npm ci` in `web/` on Windows; Linux Node dependencies cannot be reused. The lockfile includes Windows SWC packages.
7. Record actual Windows/Pi IPs, standby Wi-Fi MAC, broadcast, Windows version, adapter model, and sleep-state output. Real values and `CONTROL_TOKEN` belong in ignored local configuration, not committed examples. Keep clocks synchronized because freshness is enforced.
8. Follow the setup guide to configure the Pi, HAProxy, firewall, second laptop and startup. Stop the first primary launcher before starting presentation mode to avoid duplicate ports.

Primary presentation command, replacing the example Pi address and setting the shared control secret locally:

```powershell
powershell -NoProfile -ExecutionPolicy Bypass -File .\start-windows.ps1 -Role primary -Presentation -PiAddress 192.0.2.10
```

Standby command:

```powershell
powershell -NoProfile -ExecutionPolicy Bypass -File .\start-windows.ps1 -Role standby
```

Pi command after editing `config/pi.env`:

```sh
./start-pi.sh
```

The primary browser opens `http://127.0.0.1:3000`; Next.js talks to Pi port 8080; the generator sends requests to Pi proxy port 8088. App port 8000 and metrics port 8081 are reached by the Pi on each laptop. `localhost` is never the address of a different device.

## Integration test order and expected results

1. **Monitoring only:** leave `SCALING_ENABLED=false`. Confirm both server identities and separate histories through `/api/servers` and `/api/metrics?server=standby`. A stopped app and stopped agent must appear independently.
2. **Direct app and proxy access:** Pi can call each laptop's app and metrics; Laptop 1 can reach Pi API/proxy. HAProxy initially serves only primary. A failed ping is not conclusive because Windows may block ICMP; test HTTP.
3. **Wi-Fi wake:** designated standby sleeps, primary and Pi stay awake. Send a real packet from the Pi, repeat at least three times, and record wake-to-app-health times. Capability settings alone are not proof. Report a hardware/AP limitation if the wireless constraint cannot be met.
4. **Unattended Windows startup:** install the standby task after interactive checks, stop the interactive copy, then prove reboot and sleep/resume behavior. Do not install the standby task on Laptop 1.
5. **CPU activation:** configure actual wake/proxy settings, private operator token and `SCALING_ENABLED=true`. Begin with light bounded load and increase gradually. Four high readings five seconds apart establish the full 15 seconds; short spikes, missing, stale and repeated readings must not activate.
6. **Routing gate:** observe `STARTING_STANDBY`, successful direct health, HAProxy readiness, then `BOTH_ACTIVE`. Both server names and per-server request rates should prove distribution. A failed activation stays out of traffic and exposes a failure reason.
7. **Failure and recovery:** stop only the primary app while presentation mode remains open. After three distinct failed health probes the Pi should fail over. Verify authenticated retry, controller restart without duplicate wake, and drain/reset preserving in-flight requests. Reset requires a healthy serving primary with fresh CPU below the warning level and leaves Windows awake.
8. **AI afterward:** preserve the Pi database for 60 complete minutes of primary request history; measure capacity for the same request cost. Until actual validation, keep `AI_SCALING_ENABLED=false` and `AI_SCALING_VALIDATED=false`. Enabling flags is not evidence that model precision improved. No retraining is required for these deployment tests.

Update the hardware record with measured results and redact real network identifiers before committing it. Record Windows/PowerShell issues and fixes, test commands/results, unresolved limits, and commit status in this handoff so another session can continue accurately.

## Known behavior that can otherwise look like a bug

- The current model predicts traffic demand and surge probability, not 70% CPU. Its quality limitations remain in `model/README.md`; no 95% precision/recall claim is justified by this implementation.
- No model score is available during the 60-minute history warm-up; the CPU path still works when hardware control is enabled.
- CPU is total Windows machine usage, including the dashboard/generator on Laptop 1, not just the app process and not the Pi's CPU.
- A 2,000 RPS selection is attempted load, not guaranteed throughput. Use achieved rates, failures, HTTP 429s and latency as evidence.
- The app's default CPU-work concurrency cap is 32; the generator permits up to 256. Higher generator concurrency can intentionally cause counted 429s.
- The Pi launcher keeps the controller alive if inference fails. Windows presentation mode keeps other presentation components alive when the demo app exits; standby mode exits to let its scheduled task restart the pair.
- A full Laptop 1 shutdown removes its dashboard and generator too. Use process failure for a live presentation from that laptop, or another presentation host for full-host failure.
- HAProxy backend/server names are fixed as `demo_servers/primary` and `demo_servers/standby`; changing them requires matching code/config changes.
- `config/example.yaml` is a reference. Actual Pi runtime settings come from environment variables, normally loaded by `start-pi.sh` from ignored `config/pi.env`.
- No automatic Windows suspend/shutdown is implemented. Reset drains routing only.

## Prompt to give Windows Codex

> Read `docs/windows-codex-handoff.md` and `docs/windows-pi-setup.md`, inspect the current Git state, and continue native Windows testing of the implemented Pi-controlled wireless scaling demo. Preserve the Pi as controller/model/proxy and Laptop 1 as dashboard/load host. Run native collector and PowerShell checks first, fix measured issues, then work through the documented network, startup, CPU activation and routing tests. Keep AI shadow-only until real capacity/wake validation. Preserve existing work and the selected model; update the handoff with actual evidence and commit the fixes using SSH transport when needed.
