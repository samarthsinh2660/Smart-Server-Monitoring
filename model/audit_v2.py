"""Compare compatible frozen surge artifacts on disjoint Azure function groups."""

from __future__ import annotations

import argparse
import hashlib
import json
from pathlib import Path

from model.calibration import calibrated_probability
from model.features import FEATURE_NAMES as BASE_FEATURE_NAMES
from model.features_v2 import make_features
from model.predict import load_artifact
from model.train import classification, load_days


def digest(path: Path) -> str:
    return hashlib.sha256(path.read_bytes()).hexdigest()


def evaluate(data: dict, current: dict, previous: dict) -> dict:
    base_x = data["x"][:, :len(BASE_FEATURE_NAMES)]
    new_raw = current["classifier"].predict_proba(data["x"])[:, 1]
    new_score = calibrated_probability(current["calibrator"], new_raw)
    if previous["format"] == "serverless-demand-v2":
        old_raw = previous["classifier"].predict_proba(data["x"])[:, 1]
        old_score = calibrated_probability(previous["calibrator"], old_raw)
    else:
        old_raw = previous["classifier"].predict_proba(base_x)[:, 1]
        old_score = previous["calibrator"].predict(old_raw)
    return {
        "windows": len(data["surge"]),
        "current": classification(data["surge"], new_score, current["classifier_threshold"]),
        "previous": classification(data["surge"], old_score, previous["classifier_threshold"]),
    }


def pooled(groups: dict, version: str) -> dict:
    keys = ("true_positive", "true_negative", "false_positive", "false_negative")
    counts = {key: sum(group[version][key] for group in groups.values()) for key in keys}
    tp, tn, fp, fn = (counts[key] for key in keys)
    total = tp + tn + fp + fn
    return {
        "windows": total,
        **counts,
        "precision": tp / (tp + fp) if tp + fp else 0.0,
        "recall": tp / (tp + fn) if tp + fn else 0.0,
        "f1": 2 * tp / (2 * tp + fp + fn) if 2 * tp + fp + fn else 0.0,
        "accuracy": (tp + tn) / total,
        "no_surge_baseline_accuracy": (tn + fp) / total,
    }


def audit(input_dir: Path, current_path: Path, previous_path: Path, train_percent: int, validation_start: int) -> dict:
    current = load_artifact(current_path)
    previous = load_artifact(previous_path)
    if current["format"] != "serverless-demand-v2" or previous["format"] not in ("serverless-demand-v1", "serverless-demand-v2"):
        raise ValueError("expected active v2 and archived v1 or v2 artifact")
    validation = load_days(input_dir, (11, 12), 10, bucket_start=validation_start, feature_fn=make_features)
    result = {
        "source": "Azure Functions Trace 2019, real HTTP invocation counts",
        "current_artifact_sha256": digest(current_path),
        "previous_artifact_sha256": digest(previous_path),
        "training_days": list(range(1, 11)),
        "training_function_hash_bucket": [0, train_percent],
        "validation_days": [11, 12],
        "validation_function_hash_bucket": [validation_start, validation_start + 10],
        "test_days": [13, 14],
        "validation_recall_target": 0.60,
        "current_threshold": current["classifier_threshold"],
        "validation": evaluate(validation, current, previous),
        "test_function_buckets": {},
        "operational_scale_ready": False,
    }
    for start in (50, 60, 70, 80, 90):
        data = load_days(input_dir, (13, 14), 10, bucket_start=start, feature_fn=make_features)
        result["test_function_buckets"][f"{start}-{start+10}"] = evaluate(data, current, previous)
    result["pooled_test_buckets"] = {
        "current": pooled(result["test_function_buckets"], "current"),
        "previous": pooled(result["test_function_buckets"], "previous"),
    }
    return result


def main() -> None:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--input-dir", type=Path, default=Path("data"))
    parser.add_argument("--current", type=Path, default=Path("model/artifacts/serverless_model.joblib"))
    parser.add_argument("--previous", type=Path, default=Path("model/artifacts/serverless_model_v2_original.joblib"))
    parser.add_argument("--output", type=Path, default=Path("model/artifacts/report.json"))
    parser.add_argument("--train-percent", type=int, default=40)
    parser.add_argument("--validation-start", type=int, default=40)
    args = parser.parse_args()
    if not 1 <= args.train_percent <= args.validation_start <= 90:
        parser.error("training and validation buckets must be disjoint")
    args.output.write_text(json.dumps(audit(args.input_dir, args.current, args.previous, args.train_percent, args.validation_start), indent=2) + "\n")
    print(f"wrote {args.output}")


if __name__ == "__main__":
    main()
