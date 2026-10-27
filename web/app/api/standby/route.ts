import { NextRequest, NextResponse } from "next/server";

export async function POST(request: NextRequest): Promise<NextResponse> {
  if (request.headers.get("origin") !== request.nextUrl.origin) return NextResponse.json({ error: "Invalid request origin" }, { status: 403 });
  const token = process.env.CONTROL_TOKEN;
  if (!token) return NextResponse.json({ error: "Set CONTROL_TOKEN in the dashboard environment to enable operator actions." }, { status: 503 });
  let command: unknown;
  try { command = await request.json(); } catch { return NextResponse.json({ error: "Invalid JSON" }, { status: 400 }); }
  if (typeof command !== "object" || command === null || !("action" in command) || !["activate", "retry", "reset"].includes(String(command.action))) return NextResponse.json({ error: "Unknown standby action" }, { status: 400 });
  try {
    const url = new URL(`/api/standby/${command.action}`, process.env.CONTROLLER_API_URL ?? "http://127.0.0.1:8080");
    const response = await fetch(url, { method: "POST", cache: "no-store", headers: { Authorization: `Bearer ${token}` }, signal: AbortSignal.timeout(5000) });
    return NextResponse.json(await response.json(), { status: response.status });
  } catch { return NextResponse.json({ error: "Pi controller unavailable or command timed out. Check the state before retrying." }, { status: 503 }); }
}
