import { HistoryChart } from "./history-chart";
import { readEvents, readMetrics, readPrediction, readStatus, readTraffic, readServers, readScaling } from "./lib/controller";
import { RefreshTicker } from "./refresh-ticker";
import { TrafficLab } from "./traffic-lab";
import { ScalingControls } from "./scaling-controls";

export const dynamic = "force-dynamic";

function percent(value: number | undefined): string {
  return value === undefined ? "—" : `${value.toFixed(1)}%`;
}

function clock(value: string): string {
  const date = new Date(value);
  return Number.isNaN(date.getTime()) ? "Time unavailable" :
    `${date.toLocaleTimeString("en-GB", { timeZone: "UTC", hour: "2-digit", minute: "2-digit", second: "2-digit" })} UTC`;
}

export default async function Home() {
  const [status, metrics, traffic, events, prediction, servers, scaling, standbyMetrics, standbyTraffic] = await Promise.all([
    readStatus(), readMetrics(), readTraffic(), readEvents(), readPrediction(), readServers(), readScaling(), readMetrics("standby"), readTraffic("standby"),
  ]);
  const currentMetrics = status?.metrics_available ? metrics[0] : undefined;
  const currentTraffic = status?.traffic_available ? traffic[0] : undefined;
  const cpuHistory = [...metrics].reverse().map((sample) => sample.cpu_percent);
  const memoryHistory = [...metrics].reverse().map((sample) => sample.memory_percent);
  const requestHistory = [...traffic].reverse()
    .map((sample) => sample.request_rate_per_second)
    .filter((value): value is number => value !== null);

  return (
    <main className="page">
      <header className="topbar">
        <div className="brand">
          <div className="brand-mark" aria-hidden="true">S</div>
          <div>
            <p className="eyebrow">SMART SERVER MONITOR</p>
            <h1>System overview</h1>
          </div>
        </div>
        <div className="topbar-right">
          <RefreshTicker />
          <span className={`status-pill ${status ? "connected" : "unavailable"}`}>
            <span className="status-dot" aria-hidden="true" />
            Controller {status ? "connected" : "unavailable"}
          </span>
        </div>
      </header>

      <section className="intro" aria-labelledby="overview-heading">
        <div>
          <p className="eyebrow">PI CONTROLLER · WINDOWS SERVERS</p>
          <h2 id="overview-heading">See what your server is doing.</h2>
          <p className="subtle">Primary metrics, application traffic, and decisions from the controller API.</p>
        </div>
        <span className="phase-tag">{scaling?.enabled ? scaling.state.replaceAll("_", " ") : "Monitoring only"}</span>
      </section>

      <section className="summary-grid" aria-label="Current readings">
        <article className="summary-card">
          <p className="card-label">CPU usage</p>
          <p className="big-value">{percent(currentMetrics?.cpu_percent)}</p>
          <p className="card-caption">{currentMetrics ? `Updated ${clock(currentMetrics.sampled_at)}` : "Metrics unavailable"}</p>
        </article>
        <article className="summary-card">
          <p className="card-label">Memory usage</p>
          <p className="big-value">{percent(currentMetrics?.memory_percent)}</p>
          <p className="card-caption">{currentMetrics?.hostname ?? "Waiting for laptop agent"}</p>
        </article>
        <article className="summary-card">
          <p className="card-label">Primary request rate</p>
          <p className="big-value">{currentTraffic?.request_rate_per_second == null ? "—" : currentTraffic.request_rate_per_second.toFixed(2)}</p>
          <p className="card-caption">HTTP errors since app start: {currentTraffic?.error_count ?? "—"}</p>
        </article>
        <article className="summary-card">
          <p className="card-label">Average response</p>
          <p className="big-value">{currentTraffic?.avg_latency_ms == null ? "—" : `${currentTraffic.avg_latency_ms.toFixed(0)} ms`}</p>
          <p className="card-caption">P95: {currentTraffic?.p95_latency_ms == null ? "—" : `${currentTraffic.p95_latency_ms.toFixed(0)} ms`} · Errors: {currentTraffic?.error_rate_percent == null ? "—" : percent(currentTraffic.error_rate_percent)}</p>
        </article>
      </section>

      <section className="server-grid" aria-label="Monitored servers">
        {servers.map((server) => <article className="panel server-card" key={server.id}>
          <p className="eyebrow">{server.id === "primary" ? "LAPTOP 1 · PRIMARY" : "LAPTOP 2 · STANDBY"}</p>
          <h3>{server.metrics?.hostname ?? server.id}</h3>
          <p>App: {server.app_health.status} · Metrics: {server.metrics_available ? "available" : "unavailable"}</p>
          <p>CPU {percent(server.metrics?.cpu_percent)} · RAM {percent(server.metrics?.memory_percent)}</p>
          <p>App rate: {server.traffic_available ? (server.id === "primary" ? traffic : standbyTraffic)[0]?.request_rate_per_second?.toFixed(2) ?? "—" : "—"} requests/sec</p>
          <p>Proxy: {scaling?.targets[server.id]?.status ?? "not configured"} · Active requests: {scaling?.targets[server.id]?.sessions ?? "—"}</p>
          <HistoryChart label={`${server.id} CPU history`} values={[...(server.id === "primary" ? metrics : standbyMetrics)].reverse().map((sample) => sample.cpu_percent)} max={100} color="#167565" />
        </article>)}
      </section>

      <section className="content-grid" aria-label="Monitoring details">
        <div className="main-column">
          <section className="panel" aria-labelledby="trends-heading">
            <div className="panel-heading">
              <div>
                <p className="eyebrow">HISTORY</p>
                <h3 id="trends-heading">Recent trends</h3>
              </div>
              <span className="muted">5 second samples</span>
            </div>
            <div className="chart-row">
              <div className="chart-header"><span>CPU</span><strong>{percent(currentMetrics?.cpu_percent)}</strong></div>
              <HistoryChart label="CPU usage" values={cpuHistory} max={100} color="#167565" />
            </div>
            <div className="chart-row">
              <div className="chart-header"><span>Memory</span><strong>{percent(currentMetrics?.memory_percent)}</strong></div>
              <HistoryChart label="Memory usage" values={memoryHistory} max={100} color="#5485c5" />
            </div>
            <div className="chart-row">
              <div className="chart-header"><span>Primary requests per second</span><strong>{currentTraffic?.request_rate_per_second?.toFixed(2) ?? "—"}</strong></div>
              <HistoryChart label="Request rate" values={requestHistory} color="#bd7740" />
            </div>
            <p className="panel-note">Charts show saved history. Current values disappear when their source is unavailable.</p>
          </section>
          <TrafficLab />
        </div>

        <div className="side-column">
          <section className="panel" aria-labelledby="scaling-heading">
            <p className="eyebrow">STANDBY CONTROL</p>
            <h3 id="scaling-heading">{scaling?.state.replaceAll("_", " ") ?? "Controller unavailable"}</h3>
            <p className="prediction-detail">CPU warning {scaling?.warning_cpu_percent ?? "—"}% · activate at {scaling?.activation_cpu_percent ?? "—"}%</p>
            <p className="prediction-detail">Sustained load: {Math.min(scaling?.hold_progress_seconds ?? 0, scaling?.hold_seconds ?? 0).toFixed(0)} / {scaling?.hold_seconds ?? "—"} seconds · {scaling?.cpu_level ?? "unavailable"}</p>
            <p className="prediction-detail">{scaling?.reason ?? "Waiting for controller status"}</p>
            {scaling?.error && <p className="lab-message">{scaling.error}</p>}
            <ScalingControls status={scaling} />
            <p className="panel-note">{scaling?.enabled ? "CPU activation is enabled. The Pi waits for app health and proxy readiness before serving from standby." : "Hardware control is disabled. Configure the Pi before activation."} Predictive activation: {scaling?.enabled && scaling.ai_enabled && prediction?.operational_scale_ready ? "enabled after operator validation" : "shadow only"}. CPU thresholds trigger scaling; they do not cap usage.</p>
          </section>
          <section className="panel" aria-labelledby="prediction-heading">
            <div className="panel-heading">
              <div>
                <p className="eyebrow">AI PREVIEW</p>
                <h3 id="prediction-heading">Traffic surge forecast</h3>
              </div>
            </div>
            {prediction?.available && prediction.risk_score !== undefined ? (
              <>
                <p className="prediction-score">{Math.round(prediction.risk_score * 100)}%</p>
                <p className="prediction-detail">Chance of a traffic surge in the next {prediction.horizon_seconds ?? "—"} seconds.</p>
                <p className="prediction-detail">Forecast {prediction.forecast_rpm?.toFixed(1) ?? "—"} requests/min · upper estimate {prediction.upper_rpm?.toFixed(1) ?? "—"}</p>
                <p className="prediction-detail">Model {prediction.model_version ?? "unknown"} · trained on Azure Functions request traces</p>
                {prediction.shadow_action && prediction.shadow_action !== "capacity_unconfigured" && <p className="prediction-detail">Shadow decision: {prediction.shadow_action === "would_wake" ? "would wake standby" : "keep primary only"} at {prediction.shadow_capacity_rpm} requests/min capacity</p>}
                {prediction.decision_reason && prediction.decision_reason !== "capacity_unconfigured" && <p className="prediction-detail">Reason: {prediction.decision_reason.replaceAll("_", " ")}</p>}
                {prediction.scored_at && <p className="prediction-detail">Scored {clock(prediction.scored_at)}</p>}
              </>
            ) : (
              <p className="empty-state">Prediction unavailable: {prediction?.reason ?? "controller unavailable"}.</p>
            )}
            <p className="panel-note">Traffic forecasts need measured capacity and explicit deployment validation before the Pi uses them for early activation. The sustained CPU trigger works independently.</p>
          </section>

          <section className="panel" aria-labelledby="health-heading">
            <div className="panel-heading">
              <div>
                <p className="eyebrow">CHECKS</p>
                <h3 id="health-heading">Service health</h3>
              </div>
            </div>
            <dl className="health-list">
              <div><dt>Demo application</dt><dd className={status?.app_health.status === "healthy" ? "good-text" : "warn-text"}>{status?.app_health.status ?? "unknown"}</dd></div>
              <div><dt>Laptop metrics</dt><dd className={status?.metrics_available ? "good-text" : "warn-text"}>{status?.metrics_available ? "Available" : "Unavailable"}</dd></div>
              <div><dt>Traffic stats</dt><dd className={status?.traffic_available ? "good-text" : "warn-text"}>{status?.traffic_available ? "Available" : "Unavailable"}</dd></div>
              <div><dt>Disk used</dt><dd>{percent(currentMetrics?.disk_percent)}</dd></div>
              <div><dt>Temperature</dt><dd>{currentMetrics?.temperature_c == null ? "—" : `${currentMetrics.temperature_c.toFixed(1)} °C`}</dd></div>
              <div><dt>Hardware</dt><dd>{status?.hardware_ready ? "Ready" : "Pending"}</dd></div>
            </dl>
            <p className="panel-note">{status?.message ?? "Start the Go controller to see live status."}</p>
          </section>

          <section className="panel" aria-labelledby="events-heading">
            <div className="panel-heading">
              <div>
                <p className="eyebrow">ACTIVITY</p>
                <h3 id="events-heading">Recent events</h3>
              </div>
            </div>
            {events.length === 0 ? <p className="empty-state">No controller events yet.</p> : (
              <ol className="event-list">
                {events.map((event) => (
                  <li key={event.id}>
                    <span className={`event-dot ${event.status === "available" || event.status === "healthy" ? "good" : "warn"}`} aria-hidden="true" />
                    <div><strong>{event.message}</strong><time dateTime={event.occurred_at}>{clock(event.occurred_at)}</time></div>
                  </li>
                ))}
              </ol>
            )}
          </section>
        </div>
      </section>
      <p className="footnote">Controller state: {status?.state ?? "UNKNOWN"}. App traffic must pass through the Pi proxy for standby routing to affect the demo.</p>
    </main>
  );
}
