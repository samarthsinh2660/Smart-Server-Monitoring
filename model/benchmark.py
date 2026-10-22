"""Measure local model load, inference latency, and process memory."""

from __future__ import annotations

import argparse
import json
import platform
import resource
import statistics
import time
from pathlib import Path

from model.predict import load_artifact, score


def main() -> None:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--artifact", type=Path, default=Path("model/artifacts/serverless_model.joblib"))
    parser.add_argument("--iterations", type=int, default=100)
    args = parser.parse_args()
    if args.iterations < 2:
        parser.error("--iterations must be at least 2")

    started = time.perf_counter()
    artifact = load_artifact(args.artifact)
    load_ms = (time.perf_counter() - started) * 1000
    counts = [10] * 60
    score(artifact, counts, 600)
    timings = []
    for _ in range(args.iterations):
        started = time.perf_counter()
        score(artifact, counts, 600)
        timings.append((time.perf_counter() - started) * 1000)
    sorted_times = sorted(timings)
    print(json.dumps({
        "machine": platform.machine(),
        "python": platform.python_version(),
        "artifact_bytes": args.artifact.stat().st_size,
        "load_ms": load_ms,
        "score_median_ms": statistics.median(timings),
        "score_p95_ms": sorted_times[int(0.95 * (len(sorted_times) - 1))],
        "process_peak_rss_mib": resource.getrusage(resource.RUSAGE_SELF).ru_maxrss / 1024,
        "iterations": args.iterations,
    }, indent=2))


if __name__ == "__main__":
    main()
