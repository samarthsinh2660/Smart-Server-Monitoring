"use client";

import { useEffect, useState } from "react";

type Preset = "steady" | "busy" | "burst" | "ramp";
type LabStatus = {
  state: "idle" | "running" | "stopped" | "completed";
  preset?: Preset;
  started_at?: string;
  ends_at?: string;
  sent: number;
  succeeded: number;
  failed: number;
  cancelled: number;
  dropped: number;
};

const scenarios: { id: Preset; title: string; detail: string }[] = [
  { id: "steady", title: "Steady", detail: "2 requests/s · 2 min" },
  { id: "busy", title: "Busy", detail: "6 requests/s · 2 min" },
  { id: "burst", title: "Burst", detail: "12 requests/s · 45 sec" },
  { id: "ramp", title: "Ramp", detail: "2 → 12 requests/s · 90 sec" },
];

export function TrafficLab() {
  const [status, setStatus] = useState<LabStatus | null>(null);
  const [busy, setBusy] = useState(false);
  const [message, setMessage] = useState("");

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
        body: JSON.stringify({ action, preset }),
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
      <p className="lab-intro">Send real requests to the Go demo app. Watch the request, latency, and CPU charts above change during each run.</p>
      {running && <p className="lab-progress" role="status">Running {status?.preset} · about {remaining} seconds left</p>}
      <div className="lab-presets">
        {scenarios.map((scenario) => (
          <button key={scenario.id} type="button" className="lab-preset" disabled={!status || running || busy}
            onClick={() => { void command("start", scenario.id); }}>
            <strong>{scenario.title}</strong><span>{scenario.detail}</span>
          </button>
        ))}
      </div>
      <div className="lab-footer">
        <div className="lab-counters" aria-live="polite">
          <span>Sent <strong>{status?.sent ?? 0}</strong></span>
          <span>OK <strong>{status?.succeeded ?? 0}</strong></span>
          <span>Failed <strong>{status?.failed ?? 0}</strong></span>
          <span>Cancelled <strong>{status?.cancelled ?? 0}</strong></span>
          <span>Skipped <strong>{status?.dropped ?? 0}</strong></span>
        </div>
        <button type="button" className="lab-stop" disabled={!running || busy} onClick={() => { void command("stop"); }}>Stop traffic</button>
      </div>
      {message && <p className="lab-message" role="status">{message}</p>}
      <p className="panel-note">Preset requests are bounded and stop automatically. The AI forecast needs 60 complete minutes of history; short runs show immediate metric changes first.</p>
    </section>
  );
}
