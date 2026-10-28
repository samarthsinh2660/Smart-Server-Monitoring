"""Small local HTTP scoring service for the Python serverless model."""

from __future__ import annotations

import argparse
from collections import defaultdict
from datetime import datetime, timedelta, timezone
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
import json
import math
from pathlib import Path
from urllib.request import urlopen

from model.predict import load_artifact, score


def unavailable(reason: str) -> dict:
    return {"available": False, "reason": reason, "source": "Azure Functions Trace 2019"}


def parse_time(value: str) -> datetime:
    return datetime.fromisoformat(value.replace("Z", "+00:00")).astimezone(timezone.utc)


def counts_from_traffic(payload: dict, now: datetime) -> tuple[list[int], int]:
    if not payload.get("available"):
        raise ValueError("application traffic is unavailable")
    samples = sorted(payload.get("samples", []), key=lambda row: parse_time(row["sampled_at"]))
    if not samples or now - parse_time(samples[-1]["sampled_at"]) > timedelta(seconds=20):
        raise ValueError("application traffic is stale")
    current_start = samples[-1]["started_at"]
    restarted = any(sample["started_at"] != current_start for sample in samples)
    # A restart begins a new real history window. Keep older database rows, but
    # never mix their counters or fabricate missing minutes for this model.
    samples = [sample for sample in samples if sample["started_at"] == current_start]
    end = now.replace(second=0, microsecond=0) - timedelta(minutes=1)
    begin = end - timedelta(minutes=59)
    minute_counts: dict[datetime, int] = defaultdict(int)
    coverage: dict[datetime, int] = defaultdict(int)
    previous = None
    for sample in samples:
        moment = parse_time(sample["sampled_at"])
        bucket = moment.replace(second=0, microsecond=0)
        if previous is not None and begin <= bucket <= end:
            if sample["started_at"] != previous["started_at"]:
                raise ValueError("application restarted during traffic history")
            if moment - parse_time(previous["sampled_at"]) > timedelta(seconds=15):
                raise ValueError("traffic samples have a gap")
            delta = int(sample["request_count"]) - int(previous["request_count"])
            if delta < 0:
                raise ValueError("application request counter went backwards")
            minute_counts[bucket] += delta
            coverage[bucket] += 1
        previous = sample
    completed = sum(coverage[begin + timedelta(minutes=offset)] >= 6 for offset in range(60))
    if completed < 60:
        context = "after app restart" if restarted else "from the app"
        raise ValueError(f"AI running; collecting history {context}: {completed}/60 complete minutes")
    counts = []
    for offset in range(60):
        bucket = begin + timedelta(minutes=offset)
        counts.append(minute_counts[bucket])
    return counts, end.hour * 60 + end.minute


def make_handler(artifact: dict, controller_url: str, capacity_rpm: float | None, operational: bool = False):
    class Handler(BaseHTTPRequestHandler):
        def do_GET(self) -> None:  # noqa: N802 - stdlib interface
            if self.path == "/health":
                self.respond(200, {"status": "ok"})
                return
            if self.path != "/prediction":
                self.respond(404, {"error": "not found"})
                return
            try:
                with urlopen(controller_url.rstrip("/") + "/api/traffic?limit=1000", timeout=3) as response:
                    payload = json.load(response)
                now = datetime.now(timezone.utc)
                counts, minute = counts_from_traffic(payload, now)
                result = score(artifact, counts, minute, capacity_rpm)
                # This explicit deployment switch records the operator's completed
                # hardware/capacity validation; training metrics do not enable it.
                result["operational_scale_ready"] = operational and capacity_rpm is not None
                result["scored_at"] = now.isoformat().replace("+00:00", "Z")
            except (OSError, ValueError, KeyError, TypeError, json.JSONDecodeError) as exc:
                result = unavailable(str(exc))
            self.respond(200, result)

        def respond(self, status: int, value: dict) -> None:
            body = json.dumps(value).encode()
            self.send_response(status)
            self.send_header("Content-Type", "application/json")
            self.send_header("Content-Length", str(len(body)))
            self.end_headers()
            self.wfile.write(body)

    return Handler


def main() -> None:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--artifact", type=Path, default=Path("model/artifacts/serverless_model.joblib"))
    parser.add_argument("--controller-url", default="http://127.0.0.1:8080")
    parser.add_argument("--host", default="127.0.0.1")
    parser.add_argument("--port", type=int, default=8090)
    parser.add_argument("--capacity-rpm", type=float, help="measured one-server capacity; shadow decision only")
    parser.add_argument("--enable-operational-scaling", action="store_true", help="operator confirms physical capacity and false-wake validation; allows Pi activation")
    args = parser.parse_args()
    if args.capacity_rpm is not None and (not math.isfinite(args.capacity_rpm) or args.capacity_rpm <= 0):
        parser.error("--capacity-rpm must be positive and finite")
    if args.enable_operational_scaling and args.capacity_rpm is None:
        parser.error("--enable-operational-scaling requires a measured --capacity-rpm")
    artifact = load_artifact(args.artifact)
    server = ThreadingHTTPServer((args.host, args.port), make_handler(artifact, args.controller_url, args.capacity_rpm, args.enable_operational_scaling))
    print(f"Python model listening on {args.host}:{args.port}", flush=True)
    server.serve_forever()


if __name__ == "__main__":
    main()
