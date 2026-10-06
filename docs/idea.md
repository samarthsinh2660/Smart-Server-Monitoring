# Smart Server Monitoring and Auto-Scaling System

## Project summary

Build a two server prototype controlled by a Raspberry Pi. The Pi watches the primary laptop, wakes a standby laptop when the primary is overloaded or unavailable, waits for its application to become healthy, and then sends traffic to both servers or fails over to the standby. A dashboard records each decision and shows the current state.

This demonstrates physical scaling and failover with one standby laptop. It does not provide unlimited autoscaling or zero downtime.

## Why this is an IoT project

The Raspberry Pi is an independent edge controller connected to physical machines on a local network. It observes server and application signals, makes a local decision, sends a Wake-on-LAN (WoL) packet, and changes the physical infrastructure's operating state.

## System architecture

```mermaid
flowchart LR
    U[Client / load generator] -->|HTTP| P[Raspberry Pi]
    P --> LB[Reverse proxy]
    P --> C[Controller + dashboard API]
    C --> DB[(SQLite events + metrics)]
    C -->|poll metrics and health| A1[Primary laptop]
    C -->|WoL magic packet| A2[Standby laptop]
    C -->|enable healthy targets| LB
    LB -->|normal traffic| S1[Primary app]
    LB -->|after activation| S2[Standby app]
    A1 --- S1
    A2 --- S2
```

The Pi is the fixed entry point for clients and hosts the controller, dashboard API, and reverse proxy. Both laptops run the same stateless HTTP application in Docker. The primary runs a lightweight metrics agent. The Pi also probes the app directly so it can detect a failed laptop when its agent stops responding.

Use wired Ethernet and stable LAN addresses or DHCP reservations. The Pi and network switch/router must remain powered. The Pi is a single point of failure; making the controller highly available is outside the first build.

## Activation sequence

```mermaid
sequenceDiagram
    participant Client
    participant Pi as Pi controller and proxy
    participant Primary as Primary laptop
    participant Standby as Standby laptop
    Client->>Pi: HTTP request
    Pi->>Primary: Forward request
    loop Every 5 seconds
        Pi->>Primary: Poll metrics and GET /health
    end
    alt Sustained high load or primary failure
        Pi->>Pi: Record reason; enter STARTING_STANDBY
        Pi->>Standby: Send WoL packet
        Standby->>Standby: Boot; start Docker app automatically
        loop Until timeout
            Pi->>Standby: GET /health
        end
        Standby-->>Pi: 200 OK
        Pi->>Pi: Enable standby in proxy; record event
        Pi->>Standby: Forward requests
    else Standby does not become healthy
        Pi->>Pi: Keep standby out of proxy; alert operator
    end
```

If the primary fails, the proxy must stop routing to it as soon as failure is confirmed, even while the standby boots. Requests may fail until the standby is healthy. The standby application should start on boot through Docker's restart policy or a systemd managed Compose service. WoL only powers on the machine. The Pi must wait for a successful health probe before routing traffic to it.

## Decision policy for the first prototype

| Condition | Action |
| --- | --- |
| Primary healthy and CPU below 80% | Keep standby off; route to primary |
| Primary CPU at or above 80% for 60 seconds | Wake standby once; add it after health passes |
| Primary app health fails for 3 consecutive probes | Remove primary from proxy, wake standby, and route to standby after health passes |
| Standby boot or health check exceeds 3 minutes | Keep it out of the proxy; record failure and alert |
| Standby already starting or active | Suppress duplicate activation |

These are initial demo settings. CPU alone does not always predict user experience, so record request latency and error rate alongside CPU. Add memory pressure as a trigger only if the chosen workload needs it.

Use explicit states: `PRIMARY_ONLY`, `STARTING_STANDBY`, `BOTH_ACTIVE`, `FAILOVER`, and `ACTIVATION_FAILED`. Persist transitions and their reasons. Keep the standby active until a manual reset in the first version. Automatic power down needs a separate safety policy.

```mermaid
stateDiagram-v2
    [*] --> PRIMARY_ONLY
    PRIMARY_ONLY --> STARTING_STANDBY: load or failure
    STARTING_STANDBY --> BOTH_ACTIVE: standby healthy and primary healthy
    STARTING_STANDBY --> FAILOVER: standby healthy and primary failed
    STARTING_STANDBY --> ACTIVATION_FAILED: timeout
    BOTH_ACTIVE --> FAILOVER: primary fails
    ACTIVATION_FAILED --> STARTING_STANDBY: manual retry
    BOTH_ACTIVE --> PRIMARY_ONLY: manual reset
    FAILOVER --> PRIMARY_ONLY: primary restored and manual reset
```

## Components

| Component | Responsibility |
| --- | --- |
| Pi controller | Poll metrics and health, evaluate policy, send WoL, manage states, store events |
| Reverse proxy on Pi | Give clients one address; route only to healthy, enabled app instances |
| Primary metrics agent | Expose CPU, memory, disk, network, temperature when available, and uptime |
| Application on both laptops | Serve the same stateless HTTP endpoint and `GET /health` |
| Dashboard | Show state, recent metrics, activation timeline, reasons, and failures |
| SQLite database | Keep timestamped measurements and state changes across controller restarts |

The dashboard can poll the Pi API every few seconds. SQLite is sufficient for this prototype. A separate database server and message broker would add setup work without improving the physical activation path.

## Suggested implementation stack

- **Pi:** Raspberry Pi OS, Python controller, SQLite, and Nginx or HAProxy.
- **Laptops:** Linux or another OS with confirmed WoL support, Docker Compose, and a small Python metrics agent on the primary.
- **Application:** A tiny stateless HTTP service with `/health` and an endpoint that performs measurable work.
- **Dashboard:** A simple web page served by the Pi API. React can be added after the complete activation path works.
- **Alerts:** Dashboard events first; Telegram or another external channel is an extension.

The Pi controller owns the policy and state machine. A second backend should not independently decide when to activate the standby.

### Technology options and trade-offs

The original stack choices are all viable. For a first prototype, the simplest path is a Python controller/API on the Pi, SQLite for events, HAProxy for routing, Docker Compose on each laptop, and a basic dashboard. This keeps the physical wake and routing path small enough to finish and demonstrate.

| Area | First version | Alternative when needed |
| --- | --- | --- |
| Edge controller and metrics | Python | Go for a compact compiled service |
| Backend API | Python in the same Pi service | Node.js with TypeScript or Java with Spring Boot |
| Proxy | HAProxy | Nginx with a managed configuration update |
| Database | SQLite | PostgreSQL for a separate backend or longer retention |
| Frontend | Simple dashboard page | React with Recharts or Chart.js |
| External alerts | Dashboard event feed | Email, Telegram, Discord, or browser notifications |

If Java/Spring Boot or Node.js/TypeScript is preferred for coursework, it can serve the API and dashboard data, while a small Pi process still handles WoL and proxy changes. Keep one component responsible for the scaling decision so two services cannot issue conflicting commands.

## Hardware checks before coding

1. Confirm the standby supports WoL from the intended power state and enable it in firmware and the operating system. Some laptops cannot wake from full shutdown or on battery power.
2. Connect the standby through Ethernet, verify its MAC address, and test waking it from the Pi.
3. Verify that the standby boots into the application without a login. Measure WoL to healthy HTTP response time.
4. Assign stable LAN addresses and confirm clients can reach the Pi proxy.
5. Use a demo application whose requests are safe to run on either laptop. An application with sessions or local writes needs shared state before load balancing.

If WoL cannot wake the standby from shutdown, use the supported sleep state and state that limitation in the presentation.

## API and dashboard scope

Minimum controller API:

```http
GET  /api/status
GET  /api/metrics
GET  /api/events
POST /api/standby/activate
POST /api/standby/reset
```

Protect manual actions so only the operator can use them on the LAN. Do not expose the control API or metrics agent directly to the internet. Show each server's reachability, health, CPU and memory, active proxy targets, state, and time ordered event log.

## Build order

1. **Hardware proof:** Wake the standby from the Pi and start its app automatically on boot.
2. **Traffic path:** Put the Pi proxy in front of the primary app; add the standby manually after `/health` succeeds.
3. **Controller:** Add polling, sustained load and failure triggers, activation states, timeout, and event log.
4. **Visibility:** Add the dashboard and useful alerts.
5. **Demo polish:** Generate repeatable load, show before and after metrics, and document recovery.

## Demonstration and success criteria

Start with the primary serving requests and the standby in its tested low power state. Generate enough application load to exceed the threshold for 60 seconds. Show the Pi recording the trigger, sending WoL, waiting for the standby to boot, checking `/health`, and routing new requests to both laptops. Then simulate primary failure and show the proxy stop using it while the standby continues serving requests.

The activation path should need no manual step, and the dashboard should record timestamps and reasons for every transition. Measure **detection time**, **WoL to health time**, **total activation time**, and **request failures during failover**. Expect a boot delay; support any claim of uninterrupted service with measurements.

## Detailed monitoring design

The original monitoring idea includes CPU, RAM, disk, disk I/O, network traffic, temperature, uptime, application health, and request latency. The first implementation can collect the core signals and add the others as the hardware allows.

| Metric | Source | Display and use |
| --- | --- | --- |
| CPU utilization | Laptop agent | Main sustained load trigger and chart |
| RAM utilization | Laptop agent | Capacity warning and chart |
| Disk used and disk I/O | Laptop agent | Diagnose storage pressure |
| Network in/out | Laptop agent | Show how traffic changes after activation |
| Temperature | Laptop sensors, when supported | Hardware warning; do not assume every laptop exposes it |
| Uptime | Laptop agent | Explain boots and restarts |
| Application health | Pi directly calls `/health` | Determines proxy eligibility |
| Request rate and latency | Proxy or application | Show whether scaling improved user experience |
| Error count | Proxy or application | Show availability during failover |

Example of a timestamped sample:

```text
Time:         09:41:00
Server:       laptop-1
CPU:          91%
RAM:          84%
Disk used:    63%
Network in:   12 Mbps
Temperature:  65 C
App health:   HEALTHY
Latency p95:  420 ms
```

The Pi should poll about every 5 seconds and store server ID, timestamp, measured value, and collection result. Missing data must be shown as unavailable, not as zero. A failed metrics agent is different from a failed application: if the agent stops but `/health` still succeeds, keep serving traffic and alert that metrics are stale.

### Load levels and trigger

```text
CPU below 60%             NORMAL
CPU 60% to below 80%      WARNING
CPU 80% to below 90%      HIGH
CPU 90% or above          CRITICAL
```

These levels are for the dashboard. The actual scale trigger is CPU at or above 80% for 60 continuous seconds. Keeping display severity separate from the trigger prevents every warning from starting the standby. Save thresholds in configuration so they can be adjusted for the chosen workload. CPU is a controlled demo signal; record latency and errors to judge whether adding Laptop 2 actually helped.

## Wake-on-LAN and boot readiness

The Pi sends a WoL magic packet to the standby Ethernet adapter using its MAC address. The packet only wakes the laptop. The operating system and Docker must then start, followed by the application. The Pi must wait for the application to become healthy before changing the proxy.

```mermaid
flowchart LR
    Pi[Raspberry Pi] -->|WoL magic packet| NIC[Standby Ethernet adapter]
    NIC --> Boot[Wake and boot]
    Boot --> OS[Operating system ready]
    OS --> Docker[App container starts]
    Docker --> Health[GET /health returns 200]
    Health --> Route[Pi enables standby target]
```

Hardware checks to perform before coding the automation:

1. Verify that the standby can wake from the desired power state. Some laptops support wake from sleep but not full shutdown, and some require AC power.
2. Enable WoL in firmware and the operating system, use Ethernet, and record the standby MAC address.
3. Send a test packet from the Pi several times and check that the result is repeatable.
4. Make the application start without a user login, using Docker restart policy or a systemd managed Compose service.
5. Measure the interval from WoL send to a successful health response. Use that measurement to set a realistic startup timeout.

If shutdown wake is unsupported, use a supported sleep state and describe it in the report. This is a hardware constraint, not a failure of the software design.

## Application deployment and data consistency

Both laptops should run the same Docker image and expose the same HTTP routes. A small stateless demo app is easiest to balance: it can return its server name and perform a measurable workload. A simple `GET /health` route returns HTTP 200 only when the app is ready to serve requests.

```text
Laptop 1                         Laptop 2
- Docker                         - Docker
- Application container          - Same application image
- Metrics agent                  - App starts on boot
- /health endpoint               - /health endpoint
```

A powered-on laptop or running Docker daemon does not prove the app is ready. If the chosen application stores sessions, uploads, or other data on one laptop, requests routed to both instances may see inconsistent state. Use a stateless service for the first demo or add a shared data store as a separate extension.

## Load balancing behavior

The Pi proxy gives users one address throughout the demonstration.

```text
Normal:
Users -> Pi reverse proxy -> Laptop 1

After scale up:
                           +-> Laptop 1
Users -> Pi reverse proxy -+
                           +-> Laptop 2

After primary failure:
Users -> Pi reverse proxy -> Laptop 2
```

For overload, keep Laptop 1 in the pool if healthy and distribute new requests across both servers. For failure, remove Laptop 1 as soon as failure is confirmed, even if Laptop 2 is still booting. Requests can fail until the standby becomes healthy; measure and report that gap. HAProxy is a practical first choice because its runtime controls and health checks suit changing backend membership. Nginx is possible with a carefully managed configuration update and reload. Log which backend served each request so the demo proves that traffic actually shifted.

## Backend API details

The Pi API connects the controller, dashboard, and manual operator controls. The initial route list can be expanded from the core API as follows:

```http
GET  /api/status
GET  /api/servers
GET  /api/servers/:id
GET  /api/servers/:id/metrics
GET  /api/servers/:id/health
GET  /api/events
GET  /api/alerts

POST /api/standby/activate
POST /api/standby/retry
POST /api/standby/reset
```

The manual activate command should use the same state machine, health gate, and event logging as automatic activation. The reset command should require a healthy primary before removing the standby from the proxy. Automatic shutdown is outside the first version. Restrict mutation routes to the operator on the LAN. The metrics agent endpoint should not be publicly reachable.

Example status response:

```json
{
  "state": "BOTH_ACTIVE",
  "reason": "primary_cpu_above_80_percent_for_60_seconds",
  "activeServers": ["laptop-1", "laptop-2"],
  "standbyHealth": "healthy",
  "lastTransitionAt": "2026-10-06T09:42:21Z"
}
```

## Dashboard and alert details

The dashboard should answer three questions quickly: which laptops serve traffic, why the state changed, and whether availability or performance improved.

Show current server status, CPU/RAM/network values, historical charts, active and standby servers, application health, request latency, error counts, uptime, and a time ordered event history. Mark a measurement as stale when its last sample is old. Operator controls can provide manual activate, retry, and reset.

```text
SMART SERVER MONITOR                 State: BOTH_ACTIVE

Laptop 1  ACTIVE  HEALTHY  CPU 54%   Latency p95 120 ms
Laptop 2  ACTIVE  HEALTHY  CPU 41%   Latency p95 110 ms

09:41:00  High CPU persisted for 60 seconds
09:41:01  Wake-on-LAN sent to Laptop 2
09:42:20  Laptop 2 health check passed
09:42:21  Laptop 2 added to proxy
```

Alert examples:

| Severity | Event |
| --- | --- |
| WARNING | Primary CPU exceeded 80% |
| CRITICAL | Primary app failed three health checks; failover started |
| INFO | WoL packet sent to standby |
| SUCCESS | Standby passed health check and joined the proxy |
| ERROR | Standby did not become healthy before timeout |

The dashboard event feed is the first notification channel. Email, Telegram, Discord, or browser notifications can be added later; choose one external channel if time allows. Send alerts on state changes rather than on every polling cycle. Each event should record time, server, reason, previous state, new state, and outcome. SQLite is enough for this small prototype; PostgreSQL is a later option for larger retention or a separate backend.

## Failure handling and recovery

| Failure | Expected behavior |
| --- | --- |
| Primary application fails | Remove it from proxy, wake standby, alert |
| Primary metrics agent fails but app is healthy | Continue serving, mark metrics unavailable, alert |
| WoL packet has no effect | Wait until timeout, record activation failure, alert |
| Standby boots but app does not | Keep it out of proxy and record health failure |
| Standby fails while both are active | Remove standby; continue on healthy primary |
| Pi controller restarts | Recheck both apps and reconcile proxy targets with actual health |
| Pi or network switch fails | Service entry point is unavailable; state as prototype limitation |

Persist state transitions, but do not blindly trust saved state after a restart. The Pi should probe actual health and inspect proxy targets before resuming decisions.

## Demonstration walkthrough and measurements

1. **Normal state:** Laptop 1 serves requests, Laptop 2 is off or asleep, and the dashboard shows `PRIMARY_ONLY`.
2. **Generate load:** Run repeatable traffic against the app. Show CPU rising through the warning and high levels. Wait for the 60 second trigger window.
3. **Automatic scale up:** Show the recorded trigger, WoL packet, `STARTING_STANDBY`, successful health check, proxy change, and `BOTH_ACTIVE`. Show requests reaching both server names.
4. **Warm failure test:** With both laptops active, stop the primary app. After three failed probes, show the proxy stop routing to it and the state change to `FAILOVER`.
5. **Cold failure test:** Reset to primary only, then stop the primary. Measure failed requests while the standby boots. This demonstrates the limit of an off or sleeping standby.
6. **Failed activation test:** Prevent standby startup or health from succeeding and show timeout, `ACTIVATION_FAILED`, and an error alert.

Record detection time, WoL to health time, total activation time, request failures during cold and warm failover, and CPU/latency before and after activation. The demo succeeds when the Pi completes the activation path without manual steps, the proxy routes only to healthy servers, and the dashboard gives an accurate timeline and reason for every result.

## Extensions after the core demo

- Add latency or error rate to the trigger policy.
- Send one external notification for activation and failure.
- Add automatic scale down with a long cool down period and active request checks.
- Export metrics to Prometheus and build a richer dashboard.
- Secure remote management through a VPN.
