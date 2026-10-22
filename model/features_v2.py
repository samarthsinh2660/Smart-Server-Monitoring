"""Richer past-only minute features for the function-disjoint challenger."""

from __future__ import annotations

import numpy as np

from model.features import FEATURE_NAMES as BASE_FEATURE_NAMES, make_features as make_base_features

LAGS = (1, 2, 3, 4, 5, 7, 10, 15, 20, 30, 45, 59)
FEATURE_NAMES = BASE_FEATURE_NAMES + tuple(f"log_lag_{lag}" for lag in LAGS) + (
    "log_mean_3m",
    "log_mean_10m",
    "relative_trend_3m_10m",
    "relative_trend_10m",
    "log_max_previous_5m",
)


def make_features(counts: np.ndarray, minutes: np.ndarray) -> np.ndarray:
    base = make_base_features(counts, minutes)
    values = np.asarray(counts, dtype=np.float64)
    times = np.asarray(minutes, dtype=np.int64)
    lags = np.column_stack([np.log1p(values[times - lag]) for lag in LAGS])
    sums = np.r_[0.0, np.cumsum(values)]
    mean3 = (sums[times + 1] - sums[times - 2]) / 3
    mean10 = (sums[times + 1] - sums[times - 9]) / 10
    previous10 = (sums[times - 9] - sums[times - 19]) / 10
    extras = np.column_stack((
        np.log1p(mean3),
        np.log1p(mean10),
        (mean3 - mean10) / np.maximum(1, mean10),
        (mean10 - previous10) / np.maximum(1, mean10),
        np.log1p(np.maximum.reduce([values[times - lag] for lag in range(5, 10)])),
    ))
    return np.column_stack((base, lags, extras)).astype(np.float32)
