"""Past-only minute features shared by training and inference."""

from __future__ import annotations

import numpy as np

FEATURE_NAMES = (
    "log_latest",
    "log_mean_5m",
    "log_mean_15m",
    "log_mean_60m",
    "log_max_5m",
    "log_max_15m",
    "relative_trend_5m",
    "relative_volatility_15m",
    "active_fraction_15m",
    "minute_sin",
    "minute_cos",
)
HISTORY_MINUTES = 60
HORIZON_MINUTES = 2


def make_features(counts: np.ndarray, minutes: np.ndarray) -> np.ndarray:
    """Return features at each minute, using counts up to that minute only."""
    values = np.asarray(counts, dtype=np.float64)
    times = np.asarray(minutes, dtype=np.int64)
    if values.ndim != 1 or np.any(values < 0) or np.any(~np.isfinite(values)):
        raise ValueError("counts must be a finite nonnegative one-dimensional array")
    if np.any(times < HISTORY_MINUTES - 1) or np.any(times >= len(values)):
        raise ValueError("minute lacks a complete 60-minute history")
    sums = np.r_[0.0, np.cumsum(values)]
    squares = np.r_[0.0, np.cumsum(values * values)]

    def mean(window: int) -> np.ndarray:
        return (sums[times + 1] - sums[times + 1 - window]) / window

    mean5, mean15, mean60 = mean(5), mean(15), mean(60)
    previous5 = (sums[times - 4] - sums[times - 9]) / 5
    max5 = np.maximum.reduce([values[times - offset] for offset in range(5)])
    max15 = np.maximum.reduce([values[times - offset] for offset in range(15)])
    variance15 = (squares[times + 1] - squares[times - 14]) / 15 - mean15**2
    active = np.asarray(values > 0, dtype=np.float64)
    active_sums = np.r_[0.0, np.cumsum(active)]
    active15 = (active_sums[times + 1] - active_sums[times - 14]) / 15
    phase = 2 * np.pi * (times % 1440) / 1440
    return np.column_stack(
        (
            np.log1p(values[times]),
            np.log1p(mean5),
            np.log1p(mean15),
            np.log1p(mean60),
            np.log1p(max5),
            np.log1p(max15),
            (mean5 - previous5) / np.maximum(1, mean15),
            np.sqrt(np.maximum(variance15, 0)) / np.maximum(1, mean15),
            active15,
            np.sin(phase),
            np.cos(phase),
        )
    ).astype(np.float32)


def make_targets(counts: np.ndarray, minutes: np.ndarray) -> tuple[np.ndarray, np.ndarray]:
    values = np.asarray(counts, dtype=np.float64)
    times = np.asarray(minutes, dtype=np.int64)
    if np.any(times + HORIZON_MINUTES >= len(values)):
        raise ValueError("target extends past available data")
    peak = np.maximum(values[times + 1], values[times + 2]).astype(np.float32)
    sums = np.r_[0.0, np.cumsum(values)]
    mean5 = (sums[times + 1] - sums[times - 4]) / 5
    surge = peak >= np.maximum(20, 1.5 * mean5)
    return peak, surge.astype(np.uint8)
