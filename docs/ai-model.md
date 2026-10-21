# AI model decision and recorded-data workflow

## Decision

Use an **eight-feature logistic regression classifier written in Go** for the first Pi deployment. Train on the development laptop; load only the versioned JSON weights in the Go controller. Scoring is a few numeric operations per five-second poll and needs no Python, model server, GPU, or additional runtime. The versioned JSON model is under one kilobyte. The included synthetic model is for checking the software path, not for operational decisions.

| Option | Fit for this prototype |
| --- | --- |
| Go logistic regression | Small, deterministic, explainable, and already integrated with the controller. Selected. |
| Decision tree | Also light, but needs more branch and model-format code; useful later if recorded traces show a clear nonlinear boundary. |
| Neural network or language model | Adds runtime and tuning cost without evidence that this eight-input forecasting task needs it. |

Raspberry Pi 4 has a quad-core Cortex-A72 CPU and 1–8 GB RAM options; Pi 5 has a quad-core Cortex-A76 CPU and multiple RAM options ([Pi 4 specifications](https://www.raspberrypi.com/products/raspberry-pi-4-model-b/specifications/), [Pi 5 product brief](https://datasheets.raspberrypi.com/rpi5/raspberry-pi-5-product-brief.pdf)). Those hardware specifications support the choice of a very small CPU-only model. The exact Pi model is still unknown, so **measure inference time and controller memory on the actual board** before claiming a deployment budget.

The current model uses five-minute CPU and request-rate trends, current RAM, interval p95 latency and its trend, and error rate. P95 is estimated from bounded histogram buckets; buckets above 6.4 seconds are treated as unavailable rather than underreported. Model format 2 rejects older seven-feature weight files, so retrain after updating. A model is unavailable when the metrics or traffic input is stale, incomplete, or invalid. No prediction score may wake a machine until recorded runs and the physical health gate have been evaluated.

The prediction horizon must be measured WoL-to-healthy startup time plus margin. The current two-minute horizon is only a development default. Three consecutive scores at or above 0.8 from a recorded model write one shadow event; unavailable or low scores reset the streak, and synthetic models cannot trigger it. This event never sends WoL or changes HAProxy. The fixed CPU and app-health rules remain the planned fallback.

## Collect recorded runs

1. Start the Go services with `./start-backend.sh`. A model file is **not** required for recording.
2. Establish the matching run's low or steady baseline for five minutes before capture, so the rolling five-minute feature window contains only that run. Then generate the steady, gradual-ramp, or burst workload in a separate terminal. Keep requests flowing so request rate and latency stay available.
3. For each workload, use a unique run ID and capture longer than the prediction horizon:

   ```sh
   go run ./cmd/recordtrace -run-id steady-1 -duration 5m -horizon 2m
   go run ./cmd/recordtrace -run-id ramp-1 -duration 5m -horizon 2m
   go run ./cmd/recordtrace -run-id burst-1 -duration 5m -horizon 2m
   ```

   Run one command at a time while applying its matching workload. Rows append to ignored `data/recorded-traces.csv`. `GET /api/features` exposes the fresh feature vector and simultaneous raw CPU reading without a model.

4. The recorder labels a row positive when CPU at or above 80% has persisted for 60 seconds and that trigger falls in the next horizon. It excludes the final horizon and windows with gaps. Review positive and negative row counts: a run with no sustained overload only contributes negatives.
5. Reserve an entire run for evaluation; do not split adjacent samples from one run between training and holdout:

   ```sh
   go run ./cmd/trainmodel -input data/recorded-traces.csv -holdout-run burst-1 -version recorded-v2 -horizon-seconds 120 -output data/recorded-model.json
   ```

The trainer requires at least 12 training rows, two holdout rows, and both classes in training. It prints held-out accuracy, positive recall, confusion counts, and how often score ≥0.8 was a false alarm. These are checks, not sufficient evidence for activation: also inspect false wakes, missed overloads, lead time, and score stability on multiple new runs. Never combine synthetic and recorded rows in one model.

## Pi acceptance checks

- Measure model loading, single-score latency, and controller memory on the actual Pi while polling and serving the dashboard.
- Record standby WoL-to-healthy time and set the horizon from that measurement.
- Compare fixed-rule and prediction-assisted runs on separate workload runs; report lead time, missed overloads, unnecessary wakes, request errors, and inference cost.
- Allow prediction-assisted pre-wake only after the normal standby health check and activation state machine are implemented. A synthetic model must never enable that action.
