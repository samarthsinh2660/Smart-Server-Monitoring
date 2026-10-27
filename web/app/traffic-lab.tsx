"use client";

import { useEffect, useState, type FormEvent } from "react";

type Unit = "rpm" | "rps";
type Rate = 200 | 500 | 1000 | 2000;
type Preset = `${Unit}_${Rate | "ramp"}` | "custom";
type LabStatus = {
  state: "idle" | "running" | "stopped" | "completed";
  preset?: Preset;
  unit?: Unit;
  target_rate: number;
  achieved_rps?: number;
  started_at?: string;
  ends_at?: string;
  sent: number;
  succeeded: number;
  failed: number;
  cancelled: number;
  dropped: number;
};

const rates: Rate[] = [200, 500, 1000, 2000];

export function TrafficLab() {
  const [status, setStatus] = useState<LabStatus | null>(null);
  const [unit, setUnit] = useState<Unit>("rpm");
  const [busy, setBusy] = useState(false);
  const [message, setMessage] = useState("");
  const [custom, setCustom] = useState({ rate: 1000, concurrency: 32, workload_ms: 200, duration_seconds: 120 });

  useEffect(() => {
    let mounted = true;
    async function refresh() {
      try {
        const response = await fetch("/api/traffic-lab", { cache: "no-store" });
        if (!response.ok) throw new Error("Traffic Lab is unavailable. Start the backend first.");
        const result = await response.json() as LabStatus;
        if (mounted) { setStatus(result); setMessage(""); }
      } catch (error) {
        if (mounted) { setStatus(null); setMessage(error instanceof Error ? error.message : "Traffic Lab unavailable"); }
      }
    }
    void refresh();
    const timer = window.setInterval(() => { void refresh(); }, 2000);
    return () => { mounted = false; window.clearInterval(timer); };
  }, []);

  async function command(action: "start" | "stop", preset?: Preset) {
    setBusy(true);
    setMessage("");
    try {
      const response = await fetch("/api/traffic-lab", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ action, preset, ...(preset === "custom" ? { unit, ...custom } : {}) }),
      });
      const result = await response.json();
      if (!response.ok) throw new Error(result.error ?? "Traffic command failed");
      setStatus(result as LabStatus);
    } catch (error) {
      setMessage(error instanceof Error ? error.message : "Traffic command failed");
    } finally {
      setBusy(false);
    }
  }

  const running = status?.state === "running";
  function startCustom(event: FormEvent<HTMLFormElement>) { event.preventDefault(); void command("start", "custom"); }
  const endTime = status?.ends_at;
  const remaining = running && endTime
    ? Math.max(0, Math.ceil((new Date(endTime).getTime() - Date.now()) / 1000))
    : 0;
  return (
    <section className="panel traffic-lab" aria-labelledby="traffic-lab-heading">
      <div className="panel-heading">
        <div>
          <p className="eyebrow">FACULTY DEMO</p>
          <h3 id="traffic-lab-heading">Traffic Lab</h3>
        </div>
        <span className={`lab-state ${running ? "running" : ""}`}>{status?.state ?? "offline"}</span>
      </div>
      <form className="custom-load" onSubmit={startCustom}>
        <label>Rate ({unit})<input type="number" min="1" max="2000" step="1" required value={custom.rate} onChange={(e) => setCustom({ ...custom, rate: Number(e.target.value) })} /></label>
        <label>Concurrent requests<input type="number" min="1" max="256" required value={custom.concurrency} onChange={(e) => setCustom({ ...custom, concurrency: Number(e.target.value) })} /></label>
        <label>CPU work per request (ms)<input type="number" min="1" max="2000" required value={custom.workload_ms} onChange={(e) => setCustom({ ...custom, workload_ms: Number(e.target.value) })} /></label>
        <label>Run duration (seconds)<input type="number" min="1" max="3600" required value={custom.duration_seconds} onChange={(e) => setCustom({ ...custom, duration_seconds: Number(e.target.value) })} /></label>
        <button type="submit" disabled={!status || busy}>Apply custom load</button>
      </form>
      <p className="lab-intro">Send real requests to the Go demo app. Choose per-minute CPU load or high-volume per-second traffic, then watch the measured charts above.</p>
      <div className="lab-unit-switch" role="group" aria-label="Traffic rate unit">
        <button type="button" className={unit === "rpm" ? "selected" : ""} onClick={() => setUnit("rpm")}>Requests/minute</button>
        <button type="button" className={unit === "rps" ? "selected" : ""} onClick={() => setUnit("rps")}>Requests/second</button>
      </div>
      {running && <p className="lab-progress" role="status">Target {Math.round(status?.target_rate ?? 0).toLocaleString()} requests/{status?.unit === "rpm" ? "minute" : "second"} · about {remaining} seconds left</p>}
      <div className="lab-presets">
        {rates.map((rate) => (
          <button key={rate} type="button" className="lab-preset" disabled={!status || busy}
            onClick={() => { void command("start", `${unit}_${rate}`); }}>
            <strong>{rate.toLocaleString()} requests/{unit === "rpm" ? "min" : "sec"}</strong>
            <span>{unit === "rpm" ? "CPU work · 2 min" : "Light requests · 90 sec"}</span>
          </button>
        ))}
        <button type="button" className="lab-preset lab-ramp" disabled={!status || busy}
          onClick={() => { void command("start", `${unit}_ramp`); }}>
          <strong>Ramp 200 → 2,000 requests/{unit === "rpm" ? "min" : "sec"}</strong>
          <span>{unit === "rpm" ? "CPU work · 2 min" : "Light requests · 90 sec"}</span>
        </button>
      </div>
      <div className="lab-footer">
        <div className="lab-counters" aria-live="polite">
          <span>Sent <strong>{status?.sent ?? 0}</strong></span>
          <span>OK <strong>{status?.succeeded ?? 0}</strong></span>
          <span>Failed <strong>{status?.failed ?? 0}</strong></span>
          <span>Cancelled <strong>{status?.cancelled ?? 0}</strong></span>
          <span>Skipped <strong>{status?.dropped ?? 0}</strong></span>
          <span>Run average <strong>{status?.achieved_rps?.toFixed(1) ?? "—"} RPS</strong></span>
        </div>
        <button type="button" className="lab-stop" disabled={!running || busy} onClick={() => { void command("stop"); }}>Stop traffic</button>
      </div>
      {message && <p className="lab-message" role="status">{message}</p>}
      <p className="panel-note">Tap another rate to increase or decrease traffic during a run. Sent counts requests started; skipped counts requests blocked by the concurrency limit. The request-rate chart shows what the app handled. The AI forecast needs 60 complete minutes of history.</p>
    </section>
  );
}
