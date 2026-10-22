"""Train Python workload models on the supplied real Azure Functions trace.

Run from repository root: python3 -m model.train --input-dir data
"""

from __future__ import annotations

import argparse
import csv
import hashlib
import json
from pathlib import Path
from typing import Callable

import joblib
import numpy as np
from sklearn.ensemble import HistGradientBoostingClassifier, HistGradientBoostingRegressor
from sklearn.isotonic import IsotonicRegression
from sklearn.metrics import average_precision_score, mean_absolute_error, precision_score, recall_score

from model.features import FEATURE_NAMES, HISTORY_MINUTES, make_features, make_targets

TRAIN_DAYS = tuple(range(1, 11))
VALIDATION_DAYS = (11, 12)
TEST_DAYS = (13, 14)


def selected(function_hash: str, percent: int, bucket_start: int = 0) -> bool:
    digest = hashlib.blake2b(function_hash.encode(), digest_size=8).digest()
    return bucket_start <= int.from_bytes(digest, "big") % 100 < bucket_start + percent


def load_days(
    input_dir: Path,
    days: tuple[int, ...],
    percent: int,
    bucket_start: int = 0,
    feature_fn: Callable[[np.ndarray, np.ndarray], np.ndarray] = make_features,
) -> dict:
    if not 1 <= percent <= 100 or not 0 <= bucket_start <= 100 - percent:
        raise ValueError("invalid function hash bucket range")
    features, peaks, surges, latest = [], [], [], []
    function_days = 0
    for day in days:
        path = input_dir / f"invocations_per_function_md.anon.d{day:02d}.csv"
        if not path.is_file():
            raise FileNotFoundError(f"missing Azure invocation day {day}: {path}")
        with path.open(newline="") as stream:
            reader = csv.reader(stream)
            header = next(reader)
            if len(header) != 1444 or header[:4] != ["HashOwner", "HashApp", "HashFunction", "Trigger"] or header[-1] != "1440":
                raise ValueError(f"unexpected Azure CSV schema: {path}")
            for row in reader:
                if len(row) != 1444:
                    raise ValueError(f"incorrect field count in {path}")
                if row[3] != "http" or not selected(row[2], percent, bucket_start):
                    continue
                counts = np.fromiter((int(value) for value in row[4:]), dtype=np.int32, count=1440)
                if np.any(counts < 0):
                    raise ValueError(f"negative count in {path}")
                if counts.sum() < 300 or np.count_nonzero(counts) < 60:
                    continue
                function_days += 1
                minutes = np.arange(HISTORY_MINUTES - 1, 1438, 5)
                x = feature_fn(counts, minutes)
                peak, surge = make_targets(counts, minutes)
                features.append(x)
                peaks.append(peak)
                surges.append(surge)
                latest.append(counts[minutes].astype(np.float32))
        print(f"day {day:02d}: {function_days} cumulative selected function-days", flush=True)
    if not features:
        raise ValueError(f"no eligible HTTP workloads in days {days}")
    return {
        "x": np.concatenate(features),
        "peak": np.concatenate(peaks),
        "surge": np.concatenate(surges),
        "latest": np.concatenate(latest),
        "function_days": function_days,
    }


def classification(y: np.ndarray, probability: np.ndarray, threshold: float) -> dict:
    alert = probability >= threshold
    true_positive = int(np.count_nonzero(alert & (y == 1)))
    true_negative = int(np.count_nonzero(~alert & (y == 0)))
    false_positive = int(np.count_nonzero(alert & (y == 0)))
    false_negative = int(np.count_nonzero(~alert & (y == 1)))
    precision = float(precision_score(y, alert, zero_division=0))
    recall = float(recall_score(y, alert, zero_division=0))
    specificity = true_negative / (true_negative + false_positive) if true_negative + false_positive else 0.0
    return {
        "events": int(y.sum()),
        "alerts": int(alert.sum()),
        "true_positive": true_positive,
        "true_negative": true_negative,
        "false_positive": false_positive,
        "false_negative": false_negative,
        "false_alerts": false_positive,
        "missed_events": false_negative,
        "precision": precision,
        "recall": recall,
        "f1": 2 * precision * recall / (precision + recall) if precision + recall else 0.0,
        "accuracy": (true_positive + true_negative) / len(y),
        "balanced_accuracy": (recall + specificity) / 2,
        "no_surge_baseline_accuracy": float(np.mean(y == 0)),
        "average_precision": float(average_precision_score(y, probability)),
    }


def choose_threshold(y: np.ndarray, probability: np.ndarray) -> float:
    # Pick using validation only. The test days remain untouched until final scoring.
    thresholds = np.linspace(0.05, 0.95, 91)
    best = (-1.0, 1.0)
    for threshold in thresholds:
        scores = classification(y, probability, float(threshold))
        precision, recall = scores["precision"], scores["recall"]
        f1 = 2 * precision * recall / (precision + recall) if precision + recall else 0.0
        if f1 > best[0]:
            best = (f1, float(threshold))
    return best[1]


def main() -> None:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--input-dir", type=Path, default=Path("data"))
    parser.add_argument("--output-dir", type=Path, default=Path("model/artifacts"))
    parser.add_argument("--sample-percent", type=int, default=10)
    parser.add_argument("--force", action="store_true", help="overwrite an existing demo model")
    args = parser.parse_args()
    if not 1 <= args.sample_percent <= 100:
        parser.error("--sample-percent must be 1..100")
    if (args.output_dir / "serverless_model.joblib").exists() and not args.force:
        parser.error("demo model already exists; pass --force only if you intentionally want to retrain")

    train = load_days(args.input_dir, TRAIN_DAYS, args.sample_percent)
    validation = load_days(args.input_dir, VALIDATION_DAYS, args.sample_percent)
    print(f"fitting on {len(train['peak']):,} real trace windows", flush=True)

    forecaster = HistGradientBoostingRegressor(
        loss="absolute_error", max_iter=100, max_leaf_nodes=15,
        learning_rate=0.07, l2_regularization=2, random_state=42,
    )
    forecaster.fit(train["x"], np.log1p(train["peak"]))
    validation_raw = np.expm1(np.maximum(0, forecaster.predict(validation["x"])))
    blends = np.linspace(0, 1, 11)
    blend = min(blends, key=lambda alpha: mean_absolute_error(
        validation["peak"], alpha * validation_raw + (1 - alpha) * validation["latest"]
    ))
    validation_forecast = blend * validation_raw + (1 - blend) * validation["latest"]
    residual_p90 = float(np.quantile(
        np.log1p(validation["peak"]) - np.log1p(validation_forecast), 0.9
    ))

    classifier = HistGradientBoostingClassifier(
        max_iter=120, max_leaf_nodes=15, learning_rate=0.07,
        l2_regularization=2, random_state=42,
    )
    weights = np.where(train["surge"] == 1, 5.0, 1.0)
    classifier.fit(train["x"], train["surge"], sample_weight=weights)
    uncalibrated = classifier.predict_proba(validation["x"])[:, 1]
    calibrator = IsotonicRegression(out_of_bounds="clip")
    calibrator.fit(uncalibrated, validation["surge"])
    validation_probability = calibrator.predict(uncalibrated)
    threshold = choose_threshold(validation["surge"], validation_probability)

    # The held-out days are loaded only after all model and threshold choices are fixed.
    test = load_days(args.input_dir, TEST_DAYS, args.sample_percent)
    test_raw = np.expm1(np.maximum(0, forecaster.predict(test["x"])))
    test_forecast = blend * test_raw + (1 - blend) * test["latest"]
    test_upper = np.expm1(np.log1p(test_forecast) + residual_p90)
    test_probability = calibrator.predict(classifier.predict_proba(test["x"])[:, 1])
    test_baseline_alert = test["latest"] >= np.maximum(20, 1.5 * np.expm1(test["x"][:, 1]))

    report = {
        "dataset": "Microsoft Azure Functions Trace 2019, real HTTP invocation counts",
        "method": "past-only 60-minute features; Python histogram gradient boosting; isotonic probability calibration",
        "target": "maximum request count in next two minutes; separate surge indicator",
        "surge_definition": "future two-minute peak >= max(20 RPM, 1.5 x previous five-minute mean)",
        "sample_percent": args.sample_percent,
        "split_days": {"training": TRAIN_DAYS, "validation": VALIDATION_DAYS, "test": TEST_DAYS},
        "training_windows": len(train["peak"]),
        "validation_windows": len(validation["peak"]),
        "test_windows": len(test["peak"]),
        "function_days": {name: split["function_days"] for name, split in (("training", train), ("validation", validation), ("test", test))},
        "forecast_blend": float(blend),
        "residual_p90_log": residual_p90,
        "classifier_threshold": threshold,
        "test_forecast_mae_rpm": float(mean_absolute_error(test["peak"], test_forecast)),
        "test_persistence_mae_rpm": float(mean_absolute_error(test["peak"], test["latest"])),
        "test_upper_coverage": float(np.mean(test_upper >= test["peak"])),
        "test_surge": classification(test["surge"], test_probability, threshold),
        "test_persistence_surge": {
            "precision": float(precision_score(test["surge"], test_baseline_alert, zero_division=0)),
            "recall": float(recall_score(test["surge"], test_baseline_alert, zero_division=0)),
        },
        "operational_scale_ready": False,
        "operational_limit": "Azure invocation counts contain no target laptop capacity, latency, or second-server benefit labels",
    }
    args.output_dir.mkdir(parents=True, exist_ok=True)
    artifact = {
        "format": "serverless-demand-v1",
        "source": "Azure Functions Trace 2019",
        "feature_names": FEATURE_NAMES,
        "horizon_minutes": 2,
        "forecaster": forecaster,
        "classifier": classifier,
        "calibrator": calibrator,
        "forecast_blend": float(blend),
        "residual_p90_log": residual_p90,
        "classifier_threshold": threshold,
    }
    joblib.dump(artifact, args.output_dir / "serverless_model.joblib", compress=3)
    (args.output_dir / "report.json").write_text(json.dumps(report, indent=2) + "\n")
    print(json.dumps(report, indent=2))


if __name__ == "__main__":
    main()
