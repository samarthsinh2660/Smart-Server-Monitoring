"""Audit the archived first-generation model across function holdouts.

Run from the repository root: python3 -m model.audit --input-dir data
This script never fits or changes a model.
"""

from __future__ import annotations

import argparse
import hashlib
import json
from pathlib import Path

import numpy as np
from sklearn.metrics import precision_recall_curve

from model.predict import load_artifact
from model.train import classification, load_days


def probabilities(artifact: dict, features: np.ndarray) -> np.ndarray:
    raw = artifact["classifier"].predict_proba(features)[:, 1]
    return artifact["calibrator"].predict(raw)


def strict_threshold(labels: np.ndarray, scores: np.ndarray) -> float | None:
    precision, recall, thresholds = precision_recall_curve(labels, scores)
    alerts = np.array([np.count_nonzero(scores >= value) for value in thresholds])
    eligible = np.flatnonzero((precision[:-1] >= 0.95) & (alerts >= 50))
    if not len(eligible):
        return None
    return float(thresholds[eligible[np.argmax(recall[eligible])]])


def audit(input_dir: Path, artifact_path: Path) -> dict:
    artifact = load_artifact(artifact_path)
    if artifact["format"] != "serverless-demand-v1":
        raise ValueError("this audit requires the archived first-generation model")
    validation = load_days(input_dir, (11, 12), 10)
    validation_scores = probabilities(artifact, validation["x"])
    conservative = strict_threshold(validation["surge"], validation_scores)
    report = {
        "artifact_sha256": hashlib.sha256(artifact_path.read_bytes()).hexdigest(),
        "source": "Azure Functions Trace 2019, real HTTP invocation counts",
        "training_days": list(range(1, 11)),
        "validation_days": [11, 12],
        "test_days": [13, 14],
        "standard_threshold": float(artifact["classifier_threshold"]),
        "strict_threshold_selected_on_validation": conservative,
        "validation_strict": classification(validation["surge"], validation_scores, conservative) if conservative is not None else None,
        "test_function_buckets": {},
    }
    for start in (0, 30, 40):
        test = load_days(input_dir, (13, 14), 10, bucket_start=start)
        scores = probabilities(artifact, test["x"])
        report["test_function_buckets"][f"{start}-{start + 10}"] = {
            "windows": len(test["surge"]),
            "standard": classification(test["surge"], scores, artifact["classifier_threshold"]),
            "strict": classification(test["surge"], scores, conservative) if conservative is not None else None,
        }
    return report


def main() -> None:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--input-dir", type=Path, default=Path("data"))
    parser.add_argument("--artifact", type=Path, default=Path("model/artifacts/serverless_model_v1.joblib"))
    parser.add_argument("--output", type=Path, help="write JSON report here")
    args = parser.parse_args()
    result = json.dumps(audit(args.input_dir, args.artifact), indent=2) + "\n"
    if args.output:
        args.output.write_text(result)
    else:
        print(result, end="")


if __name__ == "__main__":
    main()
