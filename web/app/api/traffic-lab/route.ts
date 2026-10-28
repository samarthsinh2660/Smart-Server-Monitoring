import { NextRequest, NextResponse } from "next/server";
import { isSameOrigin } from "../../lib/same-origin";

export const dynamic = "force-dynamic";

const presets = new Set([
  "rpm_200", "rpm_500", "rpm_1000", "rpm_2000", "rpm_ramp",
  "rps_200", "rps_500", "rps_1000", "rps_2000", "rps_ramp",
]);

function serviceURL(path: string): URL {
  return new URL(path, process.env.TRAFFIC_LAB_URL ?? "http://127.0.0.1:8091");
}

async function forward(path: string, init?: RequestInit): Promise<NextResponse> {
  try {
    const response = await fetch(serviceURL(path), {
      ...init,
      cache: "no-store",
      signal: AbortSignal.timeout(3000),
    });
    const body = await response.json();
    return NextResponse.json(body, { status: response.status, headers: { "Cache-Control": "no-store" } });
  } catch {
    return NextResponse.json({ error: "Traffic Lab is unavailable. Start the backend first." }, { status: 503 });
  }
}

export async function GET(): Promise<NextResponse> {
  return forward("/status");
}

export async function POST(request: NextRequest): Promise<NextResponse> {
  if (!isSameOrigin(request)) {
    return NextResponse.json({ error: "Invalid request origin" }, { status: 403 });
  }
  let command: unknown;
  try {
    command = await request.json();
  } catch {
    return NextResponse.json({ error: "Invalid JSON" }, { status: 400 });
  }
  if (typeof command !== "object" || command === null) {
    return NextResponse.json({ error: "Invalid command" }, { status: 400 });
  }
  const { action, preset, unit, rate, concurrency, workload_ms, duration_seconds, continuous, user_mode, active_users } = command as Record<string, unknown>;
  if (action === "stop") return forward("/stop", { method: "POST" });
  if (action === "start" && preset === "custom") {
    const inRange = (value: unknown, max: number, integer = true) => typeof value === "number" && Number.isFinite(value) && value >= 1 && value <= max && (!integer || Number.isInteger(value));
    if (active_users !== undefined && (typeof active_users !== "number" || !Number.isInteger(active_users) || active_users < 0 || typeof concurrency !== "number" || active_users + concurrency > 256 || (user_mode === true && active_users !== 0))) return NextResponse.json({ error: "Bot concurrency plus active users must be at most 256." }, { status: 400 });
    if ((user_mode !== undefined && typeof user_mode !== "boolean") || (continuous !== undefined && typeof continuous !== "boolean") || (user_mode !== true && (!["rpm", "rps"].includes(String(unit)) || !inRange(rate, 2000, false))) || !inRange(concurrency, 256) || !inRange(workload_ms, 2000) || (continuous !== true && !inRange(duration_seconds, 3600))) return NextResponse.json({ error: "Custom load settings exceed the supported ranges." }, { status: 400 });
    return forward("/start", { method: "POST", headers: { "Content-Type": "application/json" }, body: JSON.stringify({ preset, unit, rate, concurrency, workload_ms, duration_seconds, continuous, user_mode, active_users }) });
  }
  if (action === "start" && typeof preset === "string" && presets.has(preset)) {
    return forward("/start", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ preset }),
    });
  }
  return NextResponse.json({ error: "Unknown traffic command" }, { status: 400 });
}
