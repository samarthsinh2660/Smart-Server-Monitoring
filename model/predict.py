"""Load the trained Python model and score a 60-minute request history."""

from __future__ import annotations

import argparse
import json
from pathlib import Path

import joblib
import numpy as np

from model.features import FEATURE_NAMES, HISTORY_MINUTES, make_features


def load_artifact(path: Path) -> dict:
    # joblib uses pickle. Only load artifacts produced by this project and trusted locally.
    artifact = joblib.load(path)
    if artifact.get("format") != "serverless-demand-v1" or tuple(artifact.get("feature_names", ())) != FEATURE_NAMES:
        raise ValueError("incompatible serverless model artifact")
    return artifact


def score(artifact: dict, counts: list[int] | np.ndarray, minute_of_day: int, capacity_rpm: float | None = None) -> dict:
    values = np.asarray(counts, dtype=np.float64)
    if len(values) < HISTORY_MINUTES or not 0 <= minute_of_day < 1440:
        raise ValueError("need 60 complete minutes and a minute_of_day in 0..1439")
    values = values[-HISTORY_MINUTES:]
    # The training features use time of day only for the last minute. Represent
    # the preceding 59 positions with matching modulo-1440 minute indices.
    minute = HISTORY_MINUTES - 1
    x = make_features(values, np.array([minute]))
    phase = 2 * np.pi * minute_of_day / 1440
    x[0, -2:] = (np.sin(phase), np.cos(phase))
    raw = max(0.0, float(np.expm1(max(0, artifact["forecaster"].predict(x)[0]))))
    blend = float(artifact["forecast_blend"])
    forecast = blend * raw + (1 - blend) * float(values[-1])
    upper = float(np.expm1(np.log1p(forecast) + artifact["residual_p90_log"]))
    uncalibrated = artifact["classifier"].predict_proba(x)[:, 1]
    risk = float(artifact["calibrator"].predict(uncalibrated)[0])
    if capacity_rpm is not None and (not np.isfinite(capacity_rpm) or capacity_rpm <= 0):
        raise ValueError("capacity_rpm must be a positive finite number")
    shadow_action = "capacity_unconfigured"
    if capacity_rpm is not None:
        shadow_action = "would_wake" if risk >= artifact["classifier_threshold"] and upper >= capacity_rpm else "stay_primary"
    return {
        "available": True,
        "source": "Azure Functions Trace 2019",
        "model_version": artifact["format"],
        "horizon_seconds": 120,
        "risk_score": risk,
        "forecast_rpm": forecast,
        "upper_rpm": upper,
        "surge_alert": risk >= artifact["classifier_threshold"],
        "shadow_action": shadow_action,
        "shadow_capacity_rpm": capacity_rpm,
        "operational_scale_ready": False,
    }


def main() -> None:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--artifact", type=Path, default=Path("model/artifacts/serverless_model.joblib"))
    parser.add_argument("--counts", type=Path, required=True, help="JSON array of minute request counts")
    parser.add_argument("--minute-of-day", type=int, required=True)
    parser.add_argument("--capacity-rpm", type=float, help="measured one-server capacity for a shadow-only action")
    args = parser.parse_args()
    counts = json.loads(args.counts.read_text())
    print(json.dumps(score(load_artifact(args.artifact), counts, args.minute_of_day, args.capacity_rpm), indent=2))


if __name__ == "__main__":
    main()
