export type ControllerStatus = {
  state: string;
  hardware_ready: boolean;
  message: string;
  metrics_available: boolean;
  traffic_available: boolean;
  app_health: { status: "healthy" | "unhealthy" | "unknown" };
};

export type MetricSample = {
  hostname: string;
  sampled_at: string;
  cpu_percent: number;
  memory_percent: number;
  disk_percent: number;
  temperature_c?: number;
};

export type TrafficSample = {
  sampled_at: string;
  request_count: number;
  error_count: number;
  request_rate_per_second: number | null;
  error_rate_percent: number | null;
  avg_latency_ms: number | null;
};

export type ControllerEvent = {
  id: number;
  occurred_at: string;
  kind: string;
  status: string;
  message: string;
};

function isObject(value: unknown): value is Record<string, unknown> {
  return typeof value === "object" && value !== null && !Array.isArray(value);
}

function finiteNumber(value: unknown): value is number {
  return typeof value === "number" && Number.isFinite(value);
}

function nullableNumber(value: unknown): value is number | null {
  return value === null || finiteNumber(value);
}

async function read(path: string): Promise<unknown> {
  const baseURL = process.env.CONTROLLER_API_URL ?? "http://127.0.0.1:8080";
  const response = await fetch(new URL(path, baseURL), {
    cache: "no-store",
    signal: AbortSignal.timeout(2500),
  });
  if (!response.ok) throw new Error(`Controller returned ${response.status}`);
  return response.json();
}

export async function readStatus(): Promise<ControllerStatus | null> {
  try {
    const value = await read("/api/status");
    if (
      !isObject(value) ||
      typeof value.state !== "string" ||
      typeof value.hardware_ready !== "boolean" ||
      typeof value.message !== "string" ||
      typeof value.metrics_available !== "boolean" ||
      typeof value.traffic_available !== "boolean" ||
      !isObject(value.app_health) ||
      !["healthy", "unhealthy", "unknown"].includes(String(value.app_health.status))
    ) return null;
    return value as ControllerStatus;
  } catch {
    return null;
  }
}

export async function readMetrics(): Promise<MetricSample[]> {
  try {
    const value = await read("/api/metrics?limit=60");
    if (!isObject(value) || !Array.isArray(value.samples)) return [];
    return value.samples.filter((item): item is MetricSample =>
      isObject(item) && typeof item.hostname === "string" &&
      typeof item.sampled_at === "string" && finiteNumber(item.cpu_percent) &&
      finiteNumber(item.memory_percent) && finiteNumber(item.disk_percent) &&
      (item.temperature_c === undefined || finiteNumber(item.temperature_c))
    );
  } catch {
    return [];
  }
}

export async function readTraffic(): Promise<TrafficSample[]> {
  try {
    const value = await read("/api/traffic?limit=60");
    if (!isObject(value) || !Array.isArray(value.samples)) return [];
    return value.samples.filter((item): item is TrafficSample =>
      isObject(item) && typeof item.sampled_at === "string" &&
      finiteNumber(item.request_count) && finiteNumber(item.error_count) &&
      nullableNumber(item.request_rate_per_second) &&
      nullableNumber(item.error_rate_percent) && nullableNumber(item.avg_latency_ms)
    );
  } catch {
    return [];
  }
}

export async function readEvents(): Promise<ControllerEvent[]> {
  try {
    const value = await read("/api/events?limit=12");
    if (!isObject(value) || !Array.isArray(value.events)) return [];
    return value.events.filter((item): item is ControllerEvent =>
      isObject(item) && finiteNumber(item.id) &&
      typeof item.occurred_at === "string" && typeof item.kind === "string" &&
      typeof item.status === "string" && typeof item.message === "string"
    );
  } catch {
    return [];
  }
}
