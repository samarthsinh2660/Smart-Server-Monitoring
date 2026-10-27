"""Load the trained Python model and score a 60-minute request history."""

from __future__ import annotations

import argparse
import json
from pathlib import Path

import joblib
import numpy as np

from model.features import FEATURE_NAMES, HISTORY_MINUTES, make_features
from model.features_v2 import FEATURE_NAMES as V2_FEATURE_NAMES, make_features as make_features_v2
from model.calibration import calibrated_probability


def load_artifact(path: Path) -> dict:
    # joblib uses pickle. Only load artifacts produced by this project and trusted locally.
    artifact = joblib.load(path)
    version = artifact.get("format")
    if version == "serverless-demand-v1":
        compatible = tuple(artifact.get("feature_names", ())) == FEATURE_NAMES
    elif version == "serverless-demand-v2":
        compatible = (
            tuple(artifact.get("feature_names", ())) == V2_FEATURE_NAMES
            and tuple(artifact.get("forecast_feature_names", ())) == FEATURE_NAMES
            and artifact.get("calibration_kind") == "sigmoid_logit"
        )
    else:
        compatible = False
    if not compatible:
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
    v2 = artifact["format"] == "serverless-demand-v2"
    x = make_features_v2(values, np.array([minute])) if v2 else make_features(values, np.array([minute]))
    phase = 2 * np.pi * minute_of_day / 1440
    x[0, len(FEATURE_NAMES) - 2:len(FEATURE_NAMES)] = (np.sin(phase), np.cos(phase))
    forecast_x = x[:, :len(FEATURE_NAMES)] if v2 else x
    raw = max(0.0, float(np.expm1(max(0, artifact["forecaster"].predict(forecast_x)[0]))))
    blend = float(artifact["forecast_blend"])
    forecast = blend * raw + (1 - blend) * float(values[-1])
    upper = float(np.expm1(np.log1p(forecast) + artifact["residual_p90_log"]))
    uncalibrated = artifact["classifier"].predict_proba(x)[:, 1]
    risk = float(calibrated_probability(artifact["calibrator"], uncalibrated)[0]) if v2 else float(artifact["calibrator"].predict(uncalibrated)[0])
    if capacity_rpm is not None and (not np.isfinite(capacity_rpm) or capacity_rpm <= 0):
        raise ValueError("capacity_rpm must be a positive finite number")
    shadow_action = "capacity_unconfigured"
    decision_reason = "capacity_unconfigured"
    if capacity_rpm is not None:
        if forecast >= capacity_rpm:
            shadow_action = "would_wake"
            decision_reason = "predicted_demand_reaches_capacity"
        elif risk >= artifact["classifier_threshold"] and upper >= capacity_rpm:
            shadow_action = "would_wake"
            decision_reason = "surge_risk_and_upper_demand_reach_capacity"
        else:
            shadow_action = "stay_primary"
            decision_reason = "predicted_demand_below_capacity"
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
        "decision_reason": decision_reason,
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
