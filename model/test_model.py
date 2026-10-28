from __future__ import annotations

from datetime import datetime, timedelta, timezone
from pathlib import Path
import io
from unittest.mock import patch
import unittest

import numpy as np

from model.features import make_features, make_targets
from model.features_v2 import FEATURE_NAMES as V2_FEATURE_NAMES, make_features as make_features_v2
from model.calibration import calibrated_probability
from model.predict import load_artifact, score
from model.serve import counts_from_traffic, main as serve_main
from model.train import selected


class ConstantForecaster:
    def predict(self, features):
        return np.full(len(features), np.log1p(30))


class ConstantClassifier:
    def predict_proba(self, features):
        return np.tile([0.2, 0.8], (len(features), 1))


class IdentityCalibrator:
    def predict(self, probabilities):
        return probabilities


class FeatureTests(unittest.TestCase):
    def test_function_buckets_are_disjoint(self) -> None:
        for index in range(100):
            function_hash = f"function-{index}"
            self.assertEqual(sum(selected(function_hash, 10, start) for start in range(0, 100, 10)), 1)

    def test_features_do_not_see_future(self) -> None:
        counts = np.full(70, 10)
        before = make_features(counts, np.array([59]))
        counts[60:62] = 1000
        after = make_features(counts, np.array([59]))
        np.testing.assert_array_equal(before, after)
        peak, surge = make_targets(counts, np.array([59]))
        self.assertEqual(float(peak[0]), 1000)
        self.assertEqual(int(surge[0]), 1)

    def test_requires_complete_history(self) -> None:
        with self.assertRaises(ValueError):
            make_features(np.ones(60), np.array([58]))

    def test_richer_features_do_not_see_future(self) -> None:
        counts = np.full(70, 10)
        before = make_features_v2(counts, np.array([59]))
        counts[60:62] = 1000
        after = make_features_v2(counts, np.array([59]))
        np.testing.assert_array_equal(before, after)
        self.assertEqual(before.shape[1], len(V2_FEATURE_NAMES))


class TrafficTests(unittest.TestCase):
    def test_restart_reports_real_warmup_without_mixing_counters(self) -> None:
        now = datetime(2026, 10, 22, 12, 1, 10, tzinfo=timezone.utc)
        first = datetime(2026, 10, 22, 11, 0, 55, tzinfo=timezone.utc)
        samples = []
        for index in range(722):
            sampled = first + timedelta(seconds=5 * index)
            recent = sampled >= datetime(2026, 10, 22, 11, 59, tzinfo=timezone.utc)
            samples.append({"sampled_at": sampled.isoformat(), "started_at": "2026-10-22T11:59:00Z" if recent else "2026-10-22T10:00:00Z", "request_count": index if not recent else index - 697})
        with self.assertRaisesRegex(ValueError, r"AI running; collecting history after app restart: 2/60"):
            counts_from_traffic({"available": True, "samples": samples}, now)

    def test_aggregates_complete_minute_buckets(self) -> None:
        now = datetime(2026, 10, 22, 12, 1, 10, tzinfo=timezone.utc)
        first = datetime(2026, 10, 22, 11, 0, 55, tzinfo=timezone.utc)
        samples = []
        for index in range(722):
            sampled = first + timedelta(seconds=5 * index)
            samples.append({
                "sampled_at": sampled.isoformat().replace("+00:00", "Z"),
                "started_at": "2026-10-22T10:00:00Z",
                "request_count": index,
            })
        counts, minute = counts_from_traffic({"available": True, "samples": list(reversed(samples))}, now)
        self.assertEqual(len(counts), 60)
        self.assertEqual(counts, [12] * 60)
        self.assertEqual(minute, 720)


class PolicyTests(unittest.TestCase):
    def test_operational_service_requires_measured_capacity(self) -> None:
        with patch("sys.argv", ["model.serve", "--enable-operational-scaling"]), patch("sys.stderr", new_callable=io.StringIO), patch("model.serve.load_artifact") as load:
            with self.assertRaises(SystemExit) as error:
                serve_main()
            self.assertEqual(error.exception.code, 2)
            load.assert_not_called()

    def test_v2_offline_score_matches_vectorized_model(self) -> None:
        artifact = load_artifact(Path("model/artifacts/serverless_model.joblib"))
        counts = np.full(60, 10)
        minute_of_day = 600
        features = make_features_v2(counts, np.array([59]))
        phase = 2 * np.pi * minute_of_day / 1440
        features[0, 9:11] = (np.sin(phase), np.cos(phase))
        raw = artifact["classifier"].predict_proba(features)[:, 1]
        expected = float(calibrated_probability(artifact["calibrator"], raw)[0])
        result = score(artifact, counts, minute_of_day)
        self.assertEqual(result["model_version"], "serverless-demand-v2")
        self.assertAlmostEqual(result["risk_score"], expected)
        previous = load_artifact(Path("model/artifacts/serverless_model_v1.joblib"))
        previous_result = score(previous, counts, minute_of_day)
        self.assertAlmostEqual(result["forecast_rpm"], previous_result["forecast_rpm"])
        self.assertAlmostEqual(result["upper_rpm"], previous_result["upper_rpm"])

    def test_shadow_action_never_marks_operational_ready(self) -> None:
        artifact = {
            "forecaster": ConstantForecaster(),
            "classifier": ConstantClassifier(),
            "calibrator": IdentityCalibrator(),
            "forecast_blend": 1.0,
            "residual_p90_log": 0.5,
            "classifier_threshold": 0.4,
            "format": "serverless-demand-v1",
        }
        result = score(artifact, [10] * 60, 600, capacity_rpm=40)
        self.assertEqual(result["shadow_action"], "would_wake")
        self.assertFalse(result["operational_scale_ready"])
        self.assertEqual(score(artifact, [10] * 60, 600, capacity_rpm=100)["shadow_action"], "stay_primary")


if __name__ == "__main__":
    unittest.main()
