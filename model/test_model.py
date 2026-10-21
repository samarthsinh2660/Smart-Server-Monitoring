from __future__ import annotations

from datetime import datetime, timedelta, timezone
import unittest

import numpy as np

from model.features import make_features, make_targets
from model.predict import score
from model.serve import counts_from_traffic


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


class TrafficTests(unittest.TestCase):
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
