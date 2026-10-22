"""Smooth probability calibration for the second-generation classifier."""

from __future__ import annotations

import numpy as np


def logit_features(probability: np.ndarray) -> np.ndarray:
    bounded = np.clip(np.asarray(probability, dtype=np.float64), 1e-6, 1 - 1e-6)
    return np.log(bounded / (1 - bounded)).reshape(-1, 1)


def calibrated_probability(calibrator, raw_probability: np.ndarray) -> np.ndarray:
    return calibrator.predict_proba(logit_features(raw_probability))[:, 1]
