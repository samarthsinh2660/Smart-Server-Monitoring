"""Score a frozen surge model on Huawei Public 2023 requests, without fitting.

Run from the repository root. The supplied ZIP stays under ignored data/.
"""

from __future__ import annotations

import argparse
import csv
import hashlib
import io
import json
import zipfile
from pathlib import Path

import numpy as np

from model.calibration import calibrated_probability
from model.features import HISTORY_MINUTES, make_targets
from model.features_v2 import make_features
from model.predict import load_artifact
from model.train import classification

DAY_COUNT = 26
FUNCTION_COUNT = 5093
MINUTES_PER_DAY = 1440
PREFIX = "public_dataset/csv_files/requests_minute/"


def sha256(path: Path) -> str:
    digest = hashlib.sha256()
    with path.open("rb") as stream:
        for chunk in iter(lambda: stream.read(1024 * 1024), b""):
            digest.update(chunk)
    return digest.hexdigest()


def read_day(archive: zipfile.ZipFile, day: int) -> np.ndarray:
    name = f"{PREFIX}day_{day:02d}.csv"
    counts = np.empty((MINUTES_PER_DAY, FUNCTION_COUNT), dtype=np.float32)
    with archive.open(name) as raw:
        reader = csv.reader(io.TextIOWrapper(raw, encoding="utf-8", newline=""))
        header = next(reader)
        if header != ["day", "time", *(str(i) for i in range(FUNCTION_COUNT))]:
            raise ValueError(f"unexpected Huawei columns in {name}")
        for minute, row in enumerate(reader):
            if minute >= MINUTES_PER_DAY or len(row) != FUNCTION_COUNT + 2:
                raise ValueError(f"unexpected row shape in {name}")
            if int(row[0]) != day or int(row[1]) != (day * MINUTES_PER_DAY + minute) * 60:
                raise ValueError(f"unexpected timestamp in {name} at minute {minute}")
            counts[minute] = np.fromiter(
                (float(value) if value else 0.0 for value in row[2:]),
                dtype=np.float32,
                count=FUNCTION_COUNT,
            )
        if minute != MINUTES_PER_DAY - 1:
            raise ValueError(f"incomplete Huawei day in {name}")
    if np.any(~np.isfinite(counts)) or np.any(counts < 0):
        raise ValueError(f"nonfinite or negative requests in {name}")
    return counts


def evaluate(archive_path: Path, artifact_path: Path) -> dict:
    artifact = load_artifact(artifact_path)
    if artifact["format"] != "serverless-demand-v2":
        raise ValueError("Huawei evaluation expects the current v2 feature set")
    expected = {f"{PREFIX}day_{day:02d}.csv" for day in range(DAY_COUNT)}
    minutes = np.arange(HISTORY_MINUTES - 1, MINUTES_PER_DAY - 2, 5)
    daily = []
    all_labels = []
    all_probabilities = []
    threshold = float(artifact["classifier_threshold"])
    with zipfile.ZipFile(archive_path) as archive:
        if not expected.issubset(archive.namelist()):
            raise ValueError("archive lacks one or more of the 26 Huawei days")
        for day in range(DAY_COUNT):
            counts = read_day(archive, day)
            eligible = np.flatnonzero(
                (counts.sum(axis=0) >= 300) & ((counts > 0).sum(axis=0) >= 60)
            )
            features = []
            labels = []
            for function in eligible:
                values = counts[:, function]
                features.append(make_features(values, minutes))
                labels.append(make_targets(values, minutes)[1])
            if not features:
                raise ValueError(f"no eligible functions on Huawei day {day}")
            x = np.concatenate(features)
            y = np.concatenate(labels)
            raw = artifact["classifier"].predict_proba(x)[:, 1]
            probability = calibrated_probability(artifact["calibrator"], raw)
            all_labels.append(y)
            all_probabilities.append(probability)
            scores = classification(y, probability, threshold)
            daily.append({"day": day, "eligible_function_days": len(eligible), **scores})
            print(
                f"day {day:02d}: {len(eligible)} eligible functions, "
                f"precision {scores['precision']:.3f}, recall {scores['recall']:.3f}",
                flush=True,
            )
    pooled = classification(
        np.concatenate(all_labels), np.concatenate(all_probabilities), threshold
    )
    return {
        "source": "Huawei Public Cloud production FaaS trace 2023, requests per minute",
        "source_url": "https://github.com/sir-lab/data-release/blob/main/README_data_release_2023.md",
        "archive_sha256": sha256(archive_path),
        "artifact_sha256": sha256(artifact_path),
        "artifact_threshold": threshold,
        "trained_on_huawei": False,
        "threshold_chosen_on_huawei": False,
        "days": DAY_COUNT,
        "function_columns": FUNCTION_COUNT,
        "missing_cell_assumption": "Blank request-count cells are treated as zero invocations",
        "eligibility": "at least 300 requests and 60 active minutes per function-day",
        "sampling": "every fifth minute from minute 59 through 1437, same as Azure training",
        "surge_definition": "next two-minute peak >= max(20 RPM, 1.5 x previous five-minute mean)",
        "pooled": pooled,
        "by_day": daily,
        "operational_scale_ready": False,
    }


def main() -> None:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--archive", type=Path, default=Path("data/public_dataset.zip"))
    parser.add_argument("--artifact", type=Path, default=Path("model/artifacts/serverless_model.joblib"))
    parser.add_argument("--output", type=Path, default=Path("model/artifacts/huawei_external_report.json"))
    args = parser.parse_args()
    report = evaluate(args.archive, args.artifact)
    args.output.parent.mkdir(parents=True, exist_ok=True)
    args.output.write_text(json.dumps(report, indent=2) + "\n")
    print(json.dumps(report["pooled"], indent=2))


if __name__ == "__main__":
    main()
