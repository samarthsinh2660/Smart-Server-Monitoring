export const dynamic = "force-dynamic";

type ControllerStatus = {
  state: string;
  hardware_ready: boolean;
  message: string;
};

async function readControllerStatus(): Promise<ControllerStatus | null> {
  try {
    const baseURL = process.env.CONTROLLER_API_URL ?? "http://127.0.0.1:8080";
    const response = await fetch(new URL("/api/status", baseURL), {
      cache: "no-store",
      signal: AbortSignal.timeout(2500),
    });
    if (!response.ok) return null;

    const data: unknown = await response.json();
    if (
      typeof data !== "object" ||
      data === null ||
      !("state" in data) ||
      typeof data.state !== "string" ||
      !("hardware_ready" in data) ||
      typeof data.hardware_ready !== "boolean" ||
      !("message" in data) ||
      typeof data.message !== "string"
    ) {
      return null;
    }
    return data as ControllerStatus;
  } catch {
    return null;
  }
}

export default async function Home() {
  const status = await readControllerStatus();

  return (
    <main className="page">
      <header className="topbar">
        <div className="brand-mark" aria-hidden="true">S</div>
        <div>
          <p className="eyebrow">SMART SERVER MONITORING</p>
          <h1>System overview</h1>
        </div>
      </header>

      <section className="intro" aria-labelledby="overview-heading">
        <div>
          <p className="eyebrow">LOCAL PROTOTYPE</p>
          <h2 id="overview-heading">One place to watch your servers.</h2>
          <p className="subtle">
            The Go controller owns monitoring and scaling. This dashboard shows its
            current state as the hardware setup comes online.
          </p>
        </div>
        <span className="phase-tag">Software foundation</span>
      </section>

      <section className="status-card" aria-label="Controller status">
        <div className="card-heading">
          <div>
            <p className="eyebrow">GO BACKEND</p>
            <h3>Controller status</h3>
          </div>
          <span className={`status-pill ${status ? "connected" : "unavailable"}`}>
            <span className="status-dot" aria-hidden="true" />
            {status ? "Connected" : "Unavailable"}
          </span>
        </div>
        <dl className="details">
          <div>
            <dt>State</dt>
            <dd>{status?.state ?? "UNKNOWN"}</dd>
          </div>
          <div>
            <dt>Hardware</dt>
            <dd>{status?.hardware_ready ? "Ready" : "Not ready"}</dd>
          </div>
        </dl>
        <p className="status-message">
          {status?.message ?? "The Go controller is not responding. Start it to see live status."}
        </p>
      </section>
      <p className="footnote">Hardware activation and live metrics follow in later phases.</p>
    </main>
  );
}
