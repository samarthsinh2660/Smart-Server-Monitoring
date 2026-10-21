import { HistoryChart } from "./history-chart";
import { readEvents, readMetrics, readPrediction, readStatus, readTraffic } from "./lib/controller";
import { RefreshTicker } from "./refresh-ticker";

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
  const [status, metrics, traffic, events, prediction] = await Promise.all([
    readStatus(), readMetrics(), readTraffic(), readEvents(), readPrediction(),
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
          <p className="eyebrow">LOCAL LAPTOP PROTOTYPE</p>
          <h2 id="overview-heading">See what your server is doing.</h2>
          <p className="subtle">Live laptop metrics, application traffic, and controller events in one place.</p>
        </div>
        <span className="phase-tag">Hardware setup pending</span>
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
          <p className="card-label">Request rate</p>
          <p className="big-value">{currentTraffic?.request_rate_per_second == null ? "—" : currentTraffic.request_rate_per_second.toFixed(2)}</p>
          <p className="card-caption">HTTP errors since app start: {currentTraffic?.error_count ?? "—"}</p>
        </article>
        <article className="summary-card">
          <p className="card-label">Average response</p>
          <p className="big-value">{currentTraffic?.avg_latency_ms == null ? "—" : `${currentTraffic.avg_latency_ms.toFixed(0)} ms`}</p>
          <p className="card-caption">P95: {currentTraffic?.p95_latency_ms == null ? "—" : `${currentTraffic.p95_latency_ms.toFixed(0)} ms`} · Errors: {currentTraffic?.error_rate_percent == null ? "—" : percent(currentTraffic.error_rate_percent)}</p>
        </article>
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
              <div className="chart-header"><span>Requests per second</span><strong>{currentTraffic?.request_rate_per_second?.toFixed(2) ?? "—"}</strong></div>
              <HistoryChart label="Request rate" values={requestHistory} color="#bd7740" />
            </div>
            <p className="panel-note">Charts show saved history. Current values disappear when their source is unavailable.</p>
          </section>
        </div>

        <div className="side-column">
          <section className="panel" aria-labelledby="prediction-heading">
            <div className="panel-heading">
              <div>
                <p className="eyebrow">AI PREVIEW</p>
                <h3 id="prediction-heading">Overload prediction</h3>
              </div>
            </div>
            {prediction?.available && prediction.risk_score !== undefined ? (
              <>
                <p className="prediction-score">{Math.round(prediction.risk_score * 100)}%</p>
                <p className="prediction-detail">Estimated overload risk within {prediction.horizon_seconds ?? "—"} seconds.</p>
                <p className="prediction-detail">Model {prediction.model_version ?? "unknown"} · {prediction.source === "synthetic" ? "synthetic demo data" : "recorded training data"}</p>
                {prediction.scored_at && <p className="prediction-detail">Scored {clock(prediction.scored_at)}</p>}
              </>
            ) : (
              <p className="empty-state">Prediction unavailable: {prediction?.reason ?? "controller unavailable"}.</p>
            )}
            <p className="panel-note">Preview only. The score does not wake a laptop or change traffic routing.</p>
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
      <p className="footnote">Controller state: {status?.state ?? "UNKNOWN"}. Scaling and standby activation come after hardware setup.</p>
    </main>
  );
}
