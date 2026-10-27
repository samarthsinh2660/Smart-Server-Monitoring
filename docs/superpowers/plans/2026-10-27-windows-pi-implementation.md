# Windows laptops and Pi controller implementation plan

**Goal:** Build the approved wireless two-Windows-laptop deployment with the dashboard on Laptop 1, Pi monitoring/control, a 60% CPU warning, and a 70%/15-second activation trigger.

**Architecture:** Preserve the current executables and API defaults. Add platform-specific Windows metrics, per-server storage and polling, a separately configured activation manager with persisted transitions, native magic packets, and HAProxy runtime operations. Next.js reads Pi data server-side and controls a local bounded Go generator.

**Constraints:** Go 1.22; no new runtime service or training dependency; Windows laptop binaries do not use CGO. Pi controller retains CGO SQLite. AI stays shadow-only unless explicitly enabled following validation. No automatic suspend or shutdown. No hardware actions without configured standby, proxy, and activation switch.

## Tasks and verification

- [x] Windows collection: `internal/metrics/collect_windows.go`; use native GetSystemTimes, GlobalMemoryStatusEx, GetDiskFreeSpaceExW, GetTickCount64, and GetIfEntry2. Cross-build agent and demo app for Windows amd64; actual measurements require Windows.
- [x] Per-server data: add backward-compatible `server_id` migrations and filtered reads; extend poller identity and latest sample. Test isolation, reopening, legacy migration, and failed/stale readings.
- [x] Activation: `internal/controller/scaling.go`, `internal/proxy/runtime.go`, `internal/controller/wake.go`, environment configuration. Test CPU timing and resets, duplicate suppression, unhealthy standby, failed proxy operations, restart reconciliation, and magic-packet bytes using injected adapters.
- [x] Controller API: poll optional standby; preserve primary default history and prediction routes; add `/api/servers`, `/api/scaling`, and authenticated activate/retry/reset. Surface errors and persisted transitions.
- [x] Custom load: Go validates RPM/RPS rate 1–2000, concurrency 1–256, work 1–2000 ms, and duration 1–3600 seconds. Next.js provides custom inputs and preserves presets. Test validation and cancellation boundaries.
- [x] Dashboard: show both servers, actual activation state, thresholds, hold progress, and manual controls via same-origin server routes; typecheck and build.
- [x] Deployment: separate Windows laptop/presentation and Pi launchers, Windows unattended startup, cross-build helper, HAProxy setup, complete network/configuration runbook. Build Linux services, cross-build Windows executables, run focused Go/Python regression checks, and report hardware checks separately.

## Execution

Executed inline, preserving existing work. Git metadata was read-only during the initial implementation turn. Staging and committing subsequently succeeded during the Windows handoff turn; inspect current Git state rather than assuming the earlier restriction persists.

## Verification result

Go tests pass with the race detector, including CPU hold/freshness, timestamp rejection, cancellation, failover, health/routing gates, persistence, restart recovery, guarded reset, and magic-packet validation. Windows amd64 executables and a native collector test executable cross-build with compile-time ABI guards; Windows-targeted vet passes. Eight Python model regression tests pass. A launcher process-isolation test covers failed/missing inference and cleanup. Next.js TypeScript and the webpack production build pass. Bash launcher syntax checks pass.

Windows PowerShell and native collector execution, Wi-Fi wake, actual Pi inference performance, and two-device routing remain hardware acceptance checks. No trained artifact or data was changed. Deployment uses the source checkout and Windows build scripts. The optional transfer archive was removed at the user's request; local settings, binaries, databases, and training data remain ignored by Git.
