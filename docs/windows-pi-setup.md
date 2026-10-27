# Run the Windows laptops with the Raspberry Pi

The code supports native Windows amd64 app/agent executables, two-server Pi monitoring, custom traffic, and authenticated CPU/AI-driven wake and health-gated HAProxy control. Hardware proof is separate: Windows measurements, unattended startup, Wi-Fi sleep/wake, and actual routing must be tested on your devices.

## Before switching the dual-boot laptop to Windows

Build with `./build-windows.sh`. The three Windows executables are in `bin/windows-amd64/`. A cross-built `metrics.test.exe`, when included, runs native Windows collector checks with `./metrics.test.exe -test.v`.

Copy this working project and `bin/windows-amd64/` to a drive Windows can read, or use the prepared `bin/smart-server-demo.zip` package. Extract the package on Windows and separately on the Pi. The training `data/`, Linux `node_modules`, local databases, and local secrets are not needed for this transfer. Git does not track the compiled binaries; cloning an older remote commit will not include uncommitted changes from this workspace. Do not reuse Linux Node dependencies on Windows; the presentation launcher installs Windows dependencies if needed.

Laptop prerequisites: Windows 10/11 x64, the app/agent binaries, and Node.js >=20.9 on Laptop 1 only. Go >=1.22 is optional if you use the binaries; the launcher can build them when Go is installed. No Python or SQLite setup is needed on either laptop.

## 1. Record network addresses and verify Windows wake

Use one trusted local Wi-Fi network and reserve the Pi, primary, and standby addresses if the access point allows it. These examples use documentation addresses: replace `192.0.2.10`, `.11`, and `.12` everywhere with the real Pi, primary, and standby addresses.

On the Windows standby, use PowerShell:

```powershell
powercfg /a
powercfg /devicequery wake_programmable
powercfg /devicequery wake_armed
Get-NetAdapter -Physical | Format-Table Name, InterfaceDescription, MacAddress, Status
ipconfig /all
```

Enable the Wi-Fi adapter's magic-packet wake options in Device Manager and firmware where available. Keep AC power connected and prove wake from **Sleep** first. Windows Modern Standby may expose different settings from traditional sleep; an Ethernet wake option is insufficient. [Microsoft powercfg documentation](https://learn.microsoft.com/en-us/windows-hardware/design/device-experiences/powercfg-command-line-options) and [Wi-Fi power management](https://learn.microsoft.com/en-us/windows-hardware/design/device-experiences/wi-fi-power-management-for-modern-standby-platforms) describe these capabilities. Use the connected Wi-Fi MAC consistently; randomized hardware addresses can change it.

On the Pi, use `ip -4 address` to read the actual subnet broadcast address. With `wakeonlan` installed, put the Windows standby to sleep and run:

```sh
wakeonlan -i 192.0.2.255 02:11:22:33:44:55
```

Replace both values with the measured subnet broadcast and standby Wi-Fi MAC. Repeat at least three times. If it fails, resolve Windows driver/firmware/AP support before enabling automatic wake. Phone hotspots must permit client-to-client HTTP and wake packets; a failed ping alone is inconclusive because Windows may block ICMP. Verify the app HTTP endpoint directly while awake.

## 2. Start and verify Windows services

On Laptop 2, from the extracted project folder:

```powershell
powershell -NoProfile -ExecutionPolicy Bypass -File .\start-windows.ps1 -Role standby -SkipBuild
```

This runs the native app on port 8000 and agent on 8081. Keep the terminal open for the first test; logs are in `.logs/`. On Windows itself:

```powershell
curl.exe http://127.0.0.1:8000/health
curl.exe http://127.0.0.1:8081/metrics
.\bin\windows-amd64\metrics.test.exe -test.v
```

The test executable is optional outside the prepared package. CPU is whole-machine utilization; temperature is omitted when unsupported. Compare CPU/RAM with Task Manager over several samples, allowing for the agent's shorter sampling interval.

Both laptops must allow the Pi to access app and agent ports. On **each** Windows laptop, run this in an Administrator PowerShell, replacing the Pi address:

```powershell
$piAddress = '192.0.2.10'
foreach ($port in @(8000, 8081)) {
    New-NetFirewallRule -DisplayName "SmartServer from Pi ($port)" -Direction Inbound -Protocol TCP -LocalPort $port -RemoteAddress $piAddress -Action Allow -Profile Private
}
```

The trusted Wi-Fi connection must have a Private network profile for these rules to apply. Do not disable the firewall. After both laptop services are running, confirm from the Pi:

```sh
curl -f http://192.0.2.11:8000/health
curl -f http://192.0.2.11:8081/metrics
curl -f http://192.0.2.12:8000/health
curl -f http://192.0.2.12:8081/metrics
```

Synchronize the clocks on Windows and the Pi: stale or far-future sample timestamps are rejected.

## 3. Prepare the Pi controller and model

Use a 64-bit Raspberry Pi OS. In the project on the Pi, install Go >=1.22, a C compiler, Python with venv support, and HAProxy. For Raspberry Pi OS packages:

```sh
sudo apt update
sudo apt install -y golang gcc python3-venv python3-pip haproxy wakeonlan
go version
python3 -m venv .venv
.venv/bin/python -m pip install --only-binary=:all: -r model/requirements.txt
cp config/pi.env.example config/pi.env
chmod 600 config/pi.env
```

Edit `config/pi.env` with the actual primary/standby app and agent origins, Wi-Fi MAC, and broadcast. Generate a secret with `openssl rand -hex 32` and set `CONTROL_TOKEN` to it. Keep `SCALING_ENABLED=false` initially. The YAML example is design reference only; the runtime uses environment values from `config/pi.env`.

The Python artifact is already trained. Keep `model/artifacts/serverless_model.joblib`; do not train on the Pi or copy the large Azure trace. The pinned ARM64 Python packages and native inference performance still need verification on your board. If binary wheels cannot be installed, use a compatible supported 64-bit Python rather than silently replacing the model's dependency versions.

## 4. Configure HAProxy on the Pi

Edit `deploy/haproxy/haproxy.example.cfg` locally so `primary` is the primary laptop LAN address on port 8000 and `standby` is the other laptop on port 8000. Keep backend `demo_servers` and server names `primary` and `standby`; the runtime client uses those names. Initially keep the standby's `disabled` option.

```sh
sudo install -m 644 deploy/haproxy/haproxy.example.cfg /etc/haproxy/haproxy.cfg
sudo haproxy -c -f /etc/haproxy/haproxy.cfg
sudo systemctl restart haproxy
sudo usermod -aG haproxy "$USER"
```

Open a new Pi login so the group change applies. The socket owner/group in the template permit the `haproxy` group to use `/run/haproxy/admin.sock`; do not open an unauthenticated admin socket over the network. [HAProxy's runtime API](https://docs.haproxy.org/3.0/management.html) defines the state changes and stats used by the controller.

Check `curl -f http://127.0.0.1:8088/` on the Pi. It should identify the primary. HAProxy routes workload traffic; monitoring probes go directly to each laptop's app and agent. Operator API port 8080 and proxy port 8088 must be reachable from Laptop 1 on the trusted LAN.

## 5. Start the Pi and Laptop 1 presentation

On the Pi:

```sh
./start-pi.sh
```

This starts the Go controller and Python inference when its artifact and dependencies are available; HAProxy runs separately. Missing or failed inference does not stop the controller, CPU activation, or health-failure failover. Read status from the Pi:

```sh
curl http://127.0.0.1:8080/api/servers
curl 'http://127.0.0.1:8080/api/metrics?server=standby&limit=3'
curl http://127.0.0.1:8080/api/scaling
```

On Laptop 1, set the matching operator secret and launch its app, agent, dashboard, and load generator:

```powershell
$env:CONTROL_TOKEN = 'the-same-secret-configured-on-the-Pi'
powershell -NoProfile -ExecutionPolicy Bypass -File .\start-windows.ps1 -Role primary -Presentation -PiAddress 192.0.2.10 -SkipBuild
```

Open `http://127.0.0.1:3000`. Its server reads `http://<Pi-IP>:8080`; local Traffic Lab sends application requests to `http://<Pi-IP>:8088`. Next.js operator routes attach the secret server-side; it is not sent to the browser. To start the app/agent without presenting yet, use `-Role primary -SkipBuild` without `-Presentation` and then stop that launcher before starting presentation mode.

Use fresh `npm ci` on Windows if dependencies were copied from Linux. Keep the launch terminals open; Ctrl+C stops the child processes. Presentation mode reports an exited app without stopping the dashboard, agent, or generator, so you can observe failover. Standby mode exits on a failed child so its scheduled task can restart the pair. Do not start an interactive app/agent alongside a scheduled copy using the same ports.

## 6. Install standby app startup

After the interactive standby app and agent work, use Administrator PowerShell on **Laptop 2 only**:

```powershell
powershell -NoProfile -ExecutionPolicy Bypass -File .\install-windows-startup.ps1 -PiAddress 192.0.2.10
```

Stop the interactive standby launcher, then start the task:

```powershell
Start-ScheduledTask -TaskName SmartServerStandby
Get-ScheduledTask -TaskName SmartServerStandby
```

It runs app/agent as SYSTEM after boot without login and restarts the launcher after failure. The installed task uses `-SkipBuild`; the binaries must remain in the extracted project path. Check `.logs/` and Pi health reads. Prove a reboot startup separately from a sleep/resume trial. To stop/remove it:

```powershell
Stop-ScheduledTask -TaskName SmartServerStandby
Unregister-ScheduledTask -TaskName SmartServerStandby -Confirm:$false
```

The Pi can itself use the supplied `deploy/systemd/smartserver.service` template after installing the project at `/opt/smartserver`, creating a `smartserver` account with write access to that directory, and making the account a member of `haproxy`. Adjust the template paths/user if you use another location. Do not run both the Pi service and interactive launcher at once.

## 7. Enable and demonstrate CPU activation

After the direct network checks, proxy check, Windows startup check, and Pi-to-Windows Wi-Fi wake trial pass, set `SCALING_ENABLED=true` in `config/pi.env` and restart the Pi launcher. The controller verifies the runtime socket, reconciles actual target state, and reports `PRIMARY_ONLY` when only the primary is routed.

Use Traffic Lab's custom controls to increase load gradually. Defaults are:

| Setting | Value / meaning |
| --- | --- |
| CPU warning | 60% primary whole-machine CPU |
| CPU activation | At least 70% continuously for 15 seconds of distinct fresh samples |
| Sample interval | Five seconds; four readings span fifteen seconds |
| Standby timeout | 180 seconds |
| Custom rate | 1–2,000 RPM or RPS |
| Custom concurrency | 1–256 outstanding requests |
| CPU work | 1–2,000 ms per request |
| Custom duration | 1–3,600 seconds |

The app independently allows at most 32 concurrent CPU workloads by default; extra work gets HTTP 429 and is counted as an error. `DEMOAPP_MAX_CONCURRENCY` accepts 1–256 if you intentionally change the app capacity. Custom generator concurrency above the app limit may produce 429s; capacity and latency observations must include those errors. A 70% trigger does not cap CPU usage or guarantee crash prevention.

Watch `STARTING_STANDBY`, standby health, and HAProxy readiness before `BOTH_ACTIVE`. App responses and per-server request rates prove traffic distribution. A missing/stale CPU sample resets the hold. Three distinct failed primary health checks trigger failover independently of CPU. The controller persists transitions and reconciles actual routing after restart without repeating a pending wake packet.

For an app-failure demonstration, stop only the primary `demoapp.exe` process in Task Manager while keeping the presentation launcher open. It reports the failure and keeps the dashboard, agent, and generator alive; watch the Pi fail over after three failed probes. To restore the primary app, open another PowerShell in the project, set `$env:DEMOAPP_ADDR='0.0.0.0:8000'` and `$env:SERVER_NAME='primary'`, then run `.\bin\windows-amd64\demoapp.exe`. A full Laptop 1 power-off also removes the presentation browser and local generator; use another presentation host if you need to observe that scenario live.

**Drain and reset** requires a healthy primary with fresh CPU below 60%. It drains the standby and waits for existing proxy sessions before disabling its target. It leaves Windows awake. Return the standby to the proven sleep state manually for another wake demonstration. A failed activation requires an authenticated retry or reset.

## 8. AI behavior

The model needs 60 complete minutes of primary request history and a measured `MODEL_CAPACITY_RPM` for the same CPU-work cost. Until then, its unavailability is shown explicitly while CPU activation works. Preserve the Pi database between launches.

By default, inference is a shadow preview. After measuring capacity, false wakes, lead time, and service improvement on the actual hardware, set all of the following in `config/pi.env` to permit model-assisted early activation:

- `SCALING_ENABLED=true`
- `AI_SCALING_ENABLED=true`
- `AI_SCALING_VALIDATED=true`
- `MODEL_CAPACITY_RPM` set to the measured positive capacity

The launcher passes the explicit operational switch to Python. The Go controller requires a fresh available prediction marked operational with a `would_wake` recommendation. This switch records your validation decision; it does not improve model precision or prove accuracy. Changing CPU work per request invalidates a previously measured capacity until remeasured. The 70% CPU path remains independent of prediction availability.

## Controller API

Read APIs: `/api/status`, `/api/servers`, `/api/scaling`, `/api/prediction`, `/api/events`, `/api/metrics`, `/api/metrics/latest`, and `/api/traffic`. History endpoints accept `server=primary` or `server=standby`; omitted server defaults to primary so model history never mixes laptops.

Operator actions are `POST /api/standby/activate`, `/retry`, and `/reset`, with `Authorization: Bearer <CONTROL_TOKEN>`. The dashboard exposes corresponding same-origin routes. The Pi API does not accept arbitrary shell commands, wake destinations, or proxy target names from the browser. CPU/model-triggered actions remain off unless `SCALING_ENABLED=true`.
