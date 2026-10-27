"use client";

import { useState } from "react";
import { useRouter } from "next/navigation";
import type { ScalingStatus } from "./lib/controller";

export function ScalingControls({ status }: { status: ScalingStatus | null }) {
  const [busy, setBusy] = useState(false);
  const [message, setMessage] = useState("");
  const router = useRouter();
  async function command(action: "activate" | "retry" | "reset") {
    setBusy(true); setMessage("");
    try {
      const response = await fetch("/api/standby", { method: "POST", headers: { "Content-Type": "application/json" }, body: JSON.stringify({ action }) });
      const body = await response.json();
      if (!response.ok) throw new Error(body.error ?? "Standby command failed");
      setMessage("Command accepted. Waiting for the Pi's next status update.");
      router.refresh();
    } catch (error) { setMessage(error instanceof Error ? error.message : "Standby command failed"); }
    finally { setBusy(false); }
  }
  const available = status?.enabled && status.ready && !busy;
  const mayActivate = status?.state === "PRIMARY_ONLY" || status?.state === "ACTIVATION_FAILED";
  return <div className="scaling-controls">
    <button type="button" disabled={!available || !mayActivate} onClick={() => void command(status?.state === "ACTIVATION_FAILED" ? "retry" : "activate")}>{status?.state === "ACTIVATION_FAILED" ? "Retry standby" : "Activate standby"}</button>
    <button type="button" disabled={!available || ["STARTING_STANDBY", "DRAINING_STANDBY", "PRIMARY_ONLY"].includes(status?.state ?? "")} onClick={() => void command("reset")}>Drain and reset</button>
    {message && <p role="status" className="panel-note">{message}</p>}
  </div>;
}
