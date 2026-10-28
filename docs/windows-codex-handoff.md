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

### Pi and primary deployment — 2026-10-28

**Deployment change:** the user replaced Laptop 2 with an Apple Silicon Mac and chose always-awake operation. This supersedes the earlier two-Windows/Wi-Fi-wake requirement for this demo. Windows remains native; Mac reuses its existing Docker Compose app on port 8000. Do not install Homebrew or update macOS, shell, dependencies or images merely to restart the faculty demo.

- Mac Docker Desktop and Compose were already installed; started the existing Docker engine and verified its `docker-demoapp-1` app from the Pi. Reused that container without rebuilding/replacing it. Tagged its current image `smartserver-standby:faculty-20261028`, saved `~/smartserver/standby-image.tar`, and validated `~/smartserver/compose.frozen.yml` using the original Compose project name `docker`.
- Added a native Darwin ARM64 collector/agent using built-in `top`, `vm_stat`, `sysctl`, `netstat` and `statfs`. Actual Mac parser and native metric tests passed (1.57 s native collection). Host metrics are not Docker VM metrics. Memory is a reclaimable-pages estimate; disk uses `/System/Volumes/Data`. Unsupported temperature remains omitted. Metrics agent runs as user LaunchAgent `com.smartserver.metrics`; real Pi-to-Mac `/metrics` access passed. Mac reboot/login and automatic Docker startup after reboot have not been tested.
- Pi `/api/servers` now reports both Windows and Mac app health, separate traffic and host metrics. Dashboard verified with Mac hostname, `arm64`, CPU/RAM and native-host/Docker-app labels.
- Added `STANDBY_WAKE_ENABLED` (default true). Actual Pi uses false for this always-on Mac, real standby origins and `SCALING_ENABLED=true`. Wake MAC/broadcast are unnecessary with wake disabled. Host agent URL is optional for app-only standbys; missing host metrics are explicitly unavailable. All deployed Pi Go tests passed, including always-on/no-wake health gating. HAProxy was gracefully reloaded with the real Mac target initially disabled.
- Initial live load trials (45 s and 75 s, 32 user loops) produced real fluctuating Windows CPU readings. First trial reached only about 10 s of sustained high CPU; second also did not maintain 70% for 15 s. The Pi correctly retained `PRIMARY_ONLY`; no end-to-end CPU activation success is claimed. The user elected to click authenticated **Activate standby** for a faster routing test. At last check it remained `PRIMARY_ONLY`; manual activation and actual two-host response distribution still need observation after that click. Do not lower the configured thresholds or claim successful sharing from software tests alone.
- Saved a private Pi faculty backup under `~/smartserver-faculty-backup-20261028` with controller, model, config, HAProxy config and a consistent SQLite backup. Live database and services remain in place. Local ignored `.logs/faculty-source.tar.gz`, `faculty-source.diff` and `faculty-binaries.json` preserve source and binary hashes. Caffeinate was started on Mac (`~/smartserver/caffeinate.pid`); keep its lid open.
- Latest observed AI warm-up: 41/60 complete real minutes; model service remains on Pi, operational AI remains disabled pending actual capacity/policy validation. Windows app was not restarted during Mac setup, preserving current app history.
- See [the fixed demo runbook](mac-always-on-demo.md) for quick restarts without updates/builds. Source changes remain uncommitted; credentials and actual network settings are private.

Latest steering: removed the Traffic Lab `App active users / Completed visits` display and its local `/users` link. User wants load inputs, not user-count monitoring UI. Added combined bot-rate traffic plus a separately specified persistent user group (`active_users`), with bounded combined concurrency and failure backoff. Tested Stop/cancellation in Go tests and real dashboard commands; left a small combined run of 120 RPM bot traffic plus two continuously active users (100 ms work), with zero failures in the initial live check. Model service remains running; latest-session warm-up now reports actual collected minutes (observed 9/60) instead of a generic app-restart error, without mixing counters across restarts or deleting old history. Nine Python tests passed on Pi, including a restart-history regression. Dashboard build and generator tests passed. Kept the dummy app/agent running during this update to avoid another history reset. Only the Windows generator was replaced; its current PID is recorded in ignored `.logs/traffic-lab-updated.pid` and its output uses `traffic-lab-updated` log names. This replacement is outside the original launcher's tracked child list, so stop it explicitly when ending the presentation. Pi database/model/configuration retained. User is connecting Laptop 2 to the same Wi-Fi; its address/MAC and wake/routing validation are still outstanding. SSH is optional for provisioning Windows; runtime uses HTTP and Pi-to-standby wake packets. Scaling/operational AI flags remain off until those real prerequisites can be configured and tested.

User-load follow-up: user requested that active-user controls live inside Traffic Lab, with no separate active-user/total/capacity summary cards. Implemented a custom `user_mode` that runs one sequential request loop per simulated user, with a short backoff on failed requests; `continuous` keeps it running until Stop. Existing rate presets and timed custom runs remain available. App `/stats` now carries optional live user counts; the Pi retains fresh counts in its traffic poller and exposes them through `/api/servers`. Counts become unavailable on failed/stale polling and do not require database migrations. Dashboard gets all monitoring/user counts from the Pi, never directly from the laptop. Verified the deployed Pi returns two active users and increasing completed visits; dashboard contains Traffic Lab user counts and no capacity summary card. Verified Stop returns the app to zero active users, then left a small continuous demo run of two users with 100 ms CPU work per visit. Windows app/traffic tests, deployed Pi `go test ./...`, and dashboard production build passed. Pi database, existing model, local configuration and boot services were retained for the faculty demo. The local Windows app was restarted to install the new executable, so its in-memory per-app counters restarted; the Pi's saved history remains intact.

Follow-up fix: reproduced dashboard `Invalid request origin` at `127.0.0.1`. Next's internal origin used `localhost`; both traffic and standby routes now validate the browser Origin against the actual HTTP Host and protocol. Verified both local hostnames accept same-origin requests and foreign hosts/ports return 403. Added dummy-app `/users` and root user counters: active workload requests and completed simulated visits, not unique people. User-counter observations are excluded from traffic accounting. Native app/traffic tests and dashboard production build passed. Live dashboard API trial through Pi HAProxy at 10 RPS, 200 ms work, concurrency 4 for 8 seconds completed with 79 sent, 77 succeeded, zero failed/dropped and two cancelled at the duration boundary. `/users` identified primary with two active visits during the run and zero afterward; Pi monitoring stayed healthy. Rebuilt the local Windows app and restarted presentation; Pi runtime code did not need an update for these additive app endpoints.

- Connected to the actual Pi over SSH. It runs Debian 13 ARM64, Python 3.13.5, with approximately 8 GB RAM. Installed Go 1.24.4, HAProxy, wakeonlan and required build packages.
- Deployed source and the existing selected model to `~/smartserver` on the Pi. Installed pinned ARM64 Python wheels in its `.venv`; no retraining or model replacement.
- Actual Pi benchmark (100 iterations): 197,029-byte artifact, 3,392 ms load, 13.98 ms median scoring, 14.26 ms p95, 118.64 MiB process peak RSS. All eight model regression tests passed using the venv Python. The system Python lacks those dependencies by design.
- Configured the Pi's HAProxy primary target to this Windows laptop; standby is disabled with a local placeholder until Laptop 2 is configured. Actual requests through the Pi proxy returned the primary identity.
- Installed and enabled `smartserver.service` for the Pi login account with the HAProxy group, using `~/smartserver` paths. Configuration and the shared operator token are private; Windows dashboard settings are in ignored `web/.env.local`.
- Marked the current Windows Wi-Fi connection Private and installed Pi-address-scoped rules for ports 8000 and 8081. Pi-to-Windows app health and fresh metrics passed over Wi-Fi. Sandbox-launched Windows services were initially unreachable due to the Codex inbound sandbox block; restarting the presentation launcher outside the sandbox with approval fixed this without changing the sandbox rule.
- Windows presentation is running with the Pi API/proxy destinations. Dashboard HTTP 200 and local Traffic Lab readiness passed.
- Pi controller API reports fresh primary metrics, healthy app, available traffic history, and monitoring-only state. Dashboard HTML confirms `Controller connected` and populated metrics. HAProxy socket access succeeded using the service account/group.
- Small real routing trial: Traffic Lab requested 5 RPS for 10 seconds with 10 ms work and concurrency 2 through the Pi. Sent 50, succeeded 49, failed 0, cancelled 1 at the duration boundary, dropped 0; achieved 4.9 RPS. This is a connectivity smoke test, not a capacity measurement or two-host scaling trial.
- `go test ./...` passed on the Pi for the deployed runtime source (`cmd/`, `internal/`; Traffic Lab remains on Windows). Both `haproxy` and `smartserver` are active and enabled. Boot enablement is configured; a real Pi reboot trial has not been performed.
- Pi prediction API responds with the expected `need 60 complete minutes of traffic history` reason and `operational_scale_ready:false`. Leave the Pi service/database running for history accumulation. Initial system package download was very slow over the default network path; retrying apt with `Acquire::ForceIPv4=true` completed successfully.
- `SCALING_ENABLED`, `AI_SCALING_ENABLED` and `AI_SCALING_VALIDATED` remain false. No standby address/MAC has been invented. Second-laptop startup, Wi-Fi wake, CPU activation and dual-host routing trials remain pending.
- Real network addresses and secrets remain in ignored local configuration. DHCP address changes require updating Pi settings, HAProxy, Windows firewall scope and presentation destination.

### Native Windows software setup — 2026-10-27

User limited this session to the current Windows laptop and software; Pi and second-laptop integration remain pending.

- Windows 11 x64 (10.0.26200), Go 1.26.4 and Node.js 24.16.0 are installed.
- PowerShell parser checks passed for both Windows launcher and startup installer. No scheduled task or firewall rule was installed.
- Native `go test ./internal/metrics ./internal/traffic ./demo/traffic-lab` passed.
- Built `agent.exe`, `demoapp.exe`, and `traffic-lab.exe` into ignored `bin/windows-amd64/`.
- Ran `start-windows.ps1 -Role primary -SkipBuild`: app `/health` and `/` returned healthy/primary identity; agent `/identity` and `/metrics` responded. Three distinct samples showed CPU about 32–40%, memory about 51%, disk about 68%, increasing network counters and uptime; unsupported temperature was absent. These are endpoint observations, not a synchronized Task Manager comparison.
- Installed Windows dashboard dependencies with `npm ci`. Initial audit/build detected a critical Next.js advisory at 16.4.0. Updated the lockfile to Next.js 16.4.3 using `npm update next`; subsequent `npm audit` reported zero vulnerabilities and `npm run build -- --webpack` passed, including TypeScript checks.
- Production dashboard returned HTTP 200 at `127.0.0.1:3000` without a Pi controller. Initial probes ran before startup completed; retry after readiness succeeded. This does not verify Pi data or traffic routing.
- Temporary app, agent, launcher and dashboard processes were stopped after checks. Logs remain in ignored `.logs/`. No Python installation or model changes were needed on Windows.
- Source changes are the dependency lockfile and this record; not committed or pushed during setup. All physical integration, unattended startup, Pi inference and wireless wake checks below remain pending.

To run the local app/agent again: `powershell -NoProfile -ExecutionPolicy Bypass -File .\start-windows.ps1 -Role primary -SkipBuild`. Dashboard dependencies and production build are ready; presentation mode still needs the actual Pi address and shared token.

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
