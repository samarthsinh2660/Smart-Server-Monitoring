# Predictive scaling decision

The current model is implemented in Python under [`model/`](../model/README.md). It is trained directly on the supplied real Azure Functions 2019 HTTP invocation traces. It forecasts the next two minutes of request demand and estimates surge probability. The research source, author implementation, exact training procedure, and held-out measurements are in the model guide.

The current v2 surge classifier uses richer past-only traffic features and was retrained on 3.26 million windows from 40% of Azure function hashes. A separate function group set calibration and threshold. Across five inspected evaluation groups, pooled surge precision was 45.81% and recall was 64.23%, both slightly higher than the earlier v2 artifact; one group regressed. This is exploratory evidence because the same groups have been used during model development. Neither measure reached 95%. The forecaster is the original compact v1 model. The active artifact is about 193 KB; Raspberry Pi performance remains unmeasured.

The Go controller remains responsible for monitoring, the API, and eventual device control. Its `/api/prediction` endpoint forwards the Python result. The Next.js dashboard shows traffic surge probability, forecast request rate, and model availability. With a measured `MODEL_CAPACITY_RPM`, Python reports a shadow `would_wake` or `stay_primary` recommendation using only its forecast and surge probability. The service reports `operational_scale_ready:false`; it never sends Wake-on-LAN or changes HAProxy.

## Scaling policy once hardware is available

The **model should supply the only load-based activation signal**. A model risk score must be interpreted with a measured single-server request capacity and the actual standby wake-to-healthy interval. Wake once, wait for `/health`, then add the standby to HAProxy. The app-health failure path remains separate failover behavior. Missing model inputs or stale traffic mean predictive activation is unavailable; do not turn missing data into zero demand.

This work is still pending because the Azure trace contains request counts but no CPU, latency, server capacity, startup time, or measured benefit of a second server for this project. The current surge classifier has substantial false alerts on held-out Azure days. Before activation, benchmark both laptops under the same workload, measure lead time and false wakes, and verify that routing to two machines improves latency and errors. The selected Raspberry Pi model is also unknown, so measure Python service memory and scoring time on that board.
