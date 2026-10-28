import type { NextRequest } from "next/server";

// Next's internal URL can use localhost even when the browser uses 127.0.0.1.
// Match the browser origin against the actual HTTP Host, retaining the port.
export function isSameOrigin(request: NextRequest): boolean {
  const origin = request.headers.get("origin");
  const host = request.headers.get("host");
  if (!origin || !host) return false;
  try {
    const expected = new URL(`${request.nextUrl.protocol}//${host}`);
    return new URL(origin).origin === origin && origin === expected.origin;
  } catch {
    return false;
  }
}
