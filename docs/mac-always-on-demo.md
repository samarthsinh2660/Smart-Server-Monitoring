# Faculty demo: Windows primary, Mac standby, Pi controller

The primary Windows laptop runs the native app, metrics agent, dashboard and
Traffic Lab. The always-awake Apple Silicon Mac runs the existing Docker app on
port 8000 and a native macOS metrics agent on 8081. The Pi owns the database,
model, monitoring API and HAProxy. Dashboard monitoring reads come from the Pi.

## Keep the deployed versions

Reuse the deployed executables, Docker image and installed dependencies. Do not
run dependency updates, image pulls, training, or database cleanup for the demo.
The Mac's existing app image has been tagged
`smartserver-standby:faculty-20261028` and exported to
`~/smartserver/standby-image.tar`. Its frozen Compose file is
`~/smartserver/compose.frozen.yml`. Windows stays native; Docker is only required
on the Mac. No Homebrew, shell update or macOS update is needed.

Actual addresses and operator secrets stay in local configuration. Keep the
devices on the same Wi-Fi and reserve their addresses if possible. If DHCP
changes an address, adjust the Pi origins/HAProxy, Windows firewall Pi scope,
dashboard origin and generator destination to match. Do not replace another
device's address with localhost.

## Start on the Mac

Open Docker Desktop and wait for `docker info` to succeed. Keep the lid open.
The existing Compose project is named `docker`; reuse that project so a second
app does not compete for port 8000.

```sh
open -g -a Docker
docker info
# Only if the saved image is missing:
# docker load -i "$HOME/smartserver/standby-image.tar"
docker compose -p docker -f "$HOME/smartserver/compose.frozen.yml" up -d --no-build
launchctl kickstart "gui/$(id -u)/com.smartserver.metrics"
curl -f http://127.0.0.1:8000/health
curl -f http://127.0.0.1:8081/metrics
```

The metrics LaunchAgent is installed in
`~/Library/LaunchAgents/com.smartserver.metrics.plist` and starts at user login.
For a fresh installation, replace `__PROJECT_ROOT__` in the repository template
with the deployed absolute project path and bootstrap it with `launchctl`.
Docker Desktop needs the Mac user's desktop session. Boot without login has not
been tested. Host metrics are collected with built-in macOS tools, not from
Docker's Linux VM. Memory is an estimate using physical memory minus free,
inactive and speculative pages; it is not the Activity Monitor memory-pressure
indicator. Disk monitoring targets `/System/Volumes/Data`.

To prevent idle sleep during the presentation, run `caffeinate -i` in a terminal
and leave it running, or reuse the already-started process whose PID is stored
in `~/smartserver/caffeinate.pid`. Closing the lid can still suspend the Mac.

## Start on the Pi and Windows

On the Pi, check the existing services; do not run a second controller:

```sh
systemctl is-active smartserver haproxy
systemctl is-enabled smartserver haproxy
curl -f http://127.0.0.1:8080/api/servers
curl -f http://127.0.0.1:8080/api/scaling
```

Use the existing ignored `config/pi.env`. Always-on mode has
`STANDBY_WAKE_ENABLED=false`, real standby app/agent origins, and
`SCALING_ENABLED=true`. The MAC/broadcast are unnecessary in this mode. No wake
packet or SSH command starts the Mac app; activation adds its already-running,
healthy app to routing. Both host agents are monitored separately.

On Windows, after confirming no presentation copy is already using the ports:

```powershell
powershell -NoProfile -ExecutionPolicy Bypass -File .\start-windows.ps1 -Role primary -Presentation -PiAddress <actual-Pi-IP> -SkipBuild
```

Open `http://127.0.0.1:3000`. The current session's updated traffic generator is
a separate process: its PID is in `.logs/traffic-lab-updated.pid`. Stop that
specific process before starting a fresh launcher. Do not stop unrelated Node,
Docker or Go processes.

## Present activation and traffic sharing

Start from `PRIMARY_ONLY` with both apps healthy. Select a bounded load in Traffic
Lab. Bot requests, continuous simulated users, or combined mode all send real
work through the Pi proxy. Increase gradually and observe achieved rates,
latency and failures. CPU is the Windows machine's total usage. It must remain
at least 70% for a full 15 seconds of distinct fresh samples. Short spikes do
not qualify. This trigger does not cap CPU or guarantee service capacity.

After `STARTING_STANDBY`, direct Mac health and HAProxy readiness must pass before
`BOTH_ACTIVE`. Repeated requests through Pi port 8088 identify `primary` and
`standby`; new requests use round-robin routing, not a guaranteed equal split of
CPU or existing sessions. Stop test traffic afterward. Drain/reset requires a
healthy primary below 60% and leaves the Mac app and Docker running.

The trained AI service remains on the Pi. It requires 60 complete minutes of
valid current-app history. Keep the app running and preserve the database.
Operational AI remains disabled until same-workload capacity and prediction
behavior are measured; CPU-triggered routing is independent of AI warm-up.
