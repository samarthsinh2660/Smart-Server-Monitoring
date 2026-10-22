"""Train the compact richer-history surge model on disjoint Azure functions.

The first-generation forecast model is reused; only the surge classifier changes.
Run from repository root, writing a separate reproduction artifact by default.
"""

from __future__ import annotations

import argparse
import json
from pathlib import Path

import joblib
import numpy as np
from sklearn.ensemble import HistGradientBoostingClassifier
from sklearn.linear_model import LogisticRegression
from sklearn.metrics import precision_recall_curve

from model.calibration import calibrated_probability, logit_features
from model.features import FEATURE_NAMES as FORECAST_FEATURE_NAMES
from model.features_v2 import FEATURE_NAMES, make_features
from model.predict import load_artifact
from model.train import classification, load_days

TRAIN_DAYS = tuple(range(1, 11))
VALIDATION_DAYS = (11, 12)


def recall_threshold(labels: np.ndarray, probability: np.ndarray, target_recall: float = 0.60) -> float:
    precision, recall, thresholds = precision_recall_curve(labels, probability)
    eligible = np.flatnonzero(recall[:-1] >= target_recall)
    if not len(eligible):
        raise ValueError("validation recall target cannot be met")
    return float(thresholds[eligible[np.argmax(precision[eligible])]])


def main() -> None:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--input-dir", type=Path, default=Path("data"))
    parser.add_argument("--base-artifact", type=Path, default=Path("model/artifacts/serverless_model_v1.joblib"))
    parser.add_argument("--output-dir", type=Path, default=Path("/tmp/iot-model-v2-reproduction"))
    parser.add_argument("--train-percent", type=int, default=40, help="function hash percentage from bucket zero")
    parser.add_argument("--validation-start", type=int, default=40, help="start of disjoint ten-percent calibration bucket")
    parser.add_argument("--force", action="store_true", help="overwrite a reproduction artifact")
    args = parser.parse_args()
    if not 1 <= args.train_percent <= 90 or not args.train_percent <= args.validation_start <= 90:
        parser.error("training and calibration function buckets must be disjoint and within 0..100")
    output = args.output_dir / "serverless_model.joblib"
    if output.exists() and not args.force:
        parser.error(f"artifact exists: {output}; use a new output directory or --force")

    base = load_artifact(args.base_artifact)
    if base["format"] != "serverless-demand-v1":
        parser.error("base artifact must be the archived first-generation forecast model")

    training = load_days(args.input_dir, TRAIN_DAYS, args.train_percent, feature_fn=make_features)
    validation = load_days(args.input_dir, VALIDATION_DAYS, 10, bucket_start=args.validation_start, feature_fn=make_features)
    classifier = HistGradientBoostingClassifier(
        max_iter=180, max_leaf_nodes=15, learning_rate=0.07,
        l2_regularization=2, random_state=42,
    )
    weights = np.where(training["surge"] == 1, 5.0, 1.0)
    classifier.fit(training["x"], training["surge"], sample_weight=weights)
    raw = classifier.predict_proba(validation["x"])[:, 1]
    calibrator = LogisticRegression(max_iter=1000, random_state=42)
    calibrator.fit(logit_features(raw), validation["surge"])
    probability = calibrated_probability(calibrator, raw)
    threshold = recall_threshold(validation["surge"], probability)

    artifact = dict(base)
    artifact.update({
        "format": "serverless-demand-v2",
        "feature_names": FEATURE_NAMES,
        "forecast_feature_names": FORECAST_FEATURE_NAMES,
        "classifier": classifier,
        "calibrator": calibrator,
        "calibration_kind": "sigmoid_logit",
        "classifier_threshold": threshold,
    })
    report = {
        "source": "Azure Functions Trace 2019, real HTTP invocation counts",
        "training_days": TRAIN_DAYS,
        "validation_days": VALIDATION_DAYS,
        "training_function_hash_bucket": [0, args.train_percent],
        "validation_function_hash_bucket": [args.validation_start, args.validation_start + 10],
        "training_windows": len(training["surge"]),
        "validation_windows": len(validation["surge"]),
        "validation_recall_target": 0.60,
        "classifier_threshold": threshold,
        "validation_surge": classification(validation["surge"], probability, threshold),
        "operational_scale_ready": False,
    }
    args.output_dir.mkdir(parents=True, exist_ok=True)
    joblib.dump(artifact, output, compress=3)
    (args.output_dir / "training_report.json").write_text(json.dumps(report, indent=2) + "\n")
    print(json.dumps(report, indent=2))


if __name__ == "__main__":
    main()
