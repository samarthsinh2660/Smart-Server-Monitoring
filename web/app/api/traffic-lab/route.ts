import { NextRequest, NextResponse } from "next/server";

export const dynamic = "force-dynamic";

const presets = new Set(["steady", "busy", "burst", "ramp"]);

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
  const origin = request.headers.get("origin");
  if (origin !== request.nextUrl.origin) {
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
  const { action, preset } = command as { action?: unknown; preset?: unknown };
  if (action === "stop") return forward("/stop", { method: "POST" });
  if (action === "start" && typeof preset === "string" && presets.has(preset)) {
    return forward("/start", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ preset }),
    });
  }
  return NextResponse.json({ error: "Unknown traffic command" }, { status: 400 });
}
