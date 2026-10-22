# October 2026 model quality audit

The repository already contains the full [Azure Functions 2019 archive](https://github.com/Azure/AzurePublicDataset/blob/master/AzureFunctionsDataset2019.md) under ignored `data/`: 14 daily CSVs totaling about 2 GB extracted. A second download of the same archive would not add independent examples. Only real HTTP invocation traces were used; no synthetic surge labels or laptop traces were substituted. The surge target is defined in [the model guide](README.md).

## What 95% means here

The archived v1 model's standard alert has 97.6% accuracy on its original days 13–14 sample, but the no-surge majority class alone reaches 97.9%. That alert catches 67.5% of surges with 44.8% precision. A stricter threshold was chosen using days 11–12: at least 95% observed precision and at least 50 alerts, with the highest possible validation recall among qualifying thresholds. At score 0.7761, the original sampled functions on days 13–14 show **95.5% precision, 26.9% recall, and 98.5% accuracy**. That is 700 true alerts, 33 false alerts, and 1,903 missed surges.

The same v1 model and threshold failed to transfer to functions excluded from training:

| Days 13–14 function group | Strict-alert precision | Strict-alert recall | Alerts |
| --- | ---: | ---: | ---: |
| Original 0–10% training hash bucket | 95.5% | 26.9% | 733 |
| Separate 30–40% hash bucket | 59.3% | 6.6% | 568 |
| Separate 40–50% hash bucket | 69.0% | 1.9% | 84 |

The strict threshold is **not in the deployed artifact, API, or dashboard**. Its original test result is specific to those sampled functions. Presenting it as a general 95%-precision alert would be misleading.

## XGBoost challenger

We trained a compact XGBoost classifier (300 depth-five trees, histogram algorithm, class weight five) using the same past-only features and surge label. Training used days 1–10, validation and calibration used days 11–12, and evaluation used days 13–14. One run used the original stable 10% function-hash sample. A broader run used 30% of function hashes and 2,321,712 training windows. The broader model's validation F1 threshold was chosen before examining days 13–14. The classifier used isotonic calibration when scored as a probability.

| Evaluation group on days 13–14 | V1 precision | V1 recall | Broader XGBoost precision | Broader XGBoost recall |
| --- | ---: | ---: | ---: | ---: |
| Original 10% function sample | 44.8% | 67.5% | 44.6% | 63.7% |
| Separate 30–40% hash bucket | 38.6% | 48.9% | 46.9% | 56.6% |
| Separate 40–50% hash bucket | 38.2% | 46.6% | 41.4% | 42.7% |

The 40–50% bucket was checked only after the 30–40% exploratory comparison. It confirms a precision gain but a recall loss, so this challenger did not consistently improve both measures. Its newly trained demand forecaster also selected a zero model blend on validation, leaving only a latest-minute baseline. We retained the earlier forecaster, which had a 3.88 versus 4.31 requests/minute error on its original held-out sample. XGBoost is absent from the active Pi inference stack.

## Smaller richer-history model selected for the demo

The active v2 classifier uses 12 additional past-minute lags and short-window trends. It is another small scikit-learn histogram-gradient-boosted model. Days 1–10 from function-hash bucket 0–20% fit it; days 11–12 from disjoint bucket 20–30% set a smooth probability calibration and the highest-precision threshold with at least 60% validation recall. The frozen v1 demand forecaster remains in the active bundle. Its classifier did **not** train on any evaluation function group below.

| Days 13–14 function group | V1 precision / recall | V2 precision / recall | V2 F1 |
| --- | ---: | ---: | ---: |
| 50–60% | 50.7% / 69.1% | 52.6% / 71.7% | 60.7% |
| 60–70% | 35.6% / 53.9% | 41.2% / 57.8% | 48.1% |
| 70–80% | 39.0% / 41.5% | 49.2% / 61.4% | 54.6% |
| 80–90% | 42.7% / 58.6% | 42.7% / 62.2% | 50.7% |
| 90–100% | 40.2% / 55.9% | 40.4% / 60.9% | 48.6% |

Precision and recall improved in every listed group, although the precision gain is negligible in the final two. Several groups were viewed while developing the model; the 90–100% group was checked after the final calibration choice. Further tuning against these same days would invalidate their role as a holdout. The full [v2 report](artifacts/report.json) gives confusion matrices and accuracy.

Across all five groups, v2 precision is **45.6%** versus **42.5%** for v1, and recall is **63.5%** versus **56.4%**. F1 rises from **48.4%** to **53.0%**. These pooled figures count overlapping time windows within functions, so they should not be read as independent confidence intervals.

This audit does **not** show 95% precision and recall together. A fresh external workload and measurements of real server capacity, wake time, latency, and failed requests are needed before claiming reliable predictive scaling. Accuracy alone cannot establish that quality.

## Broader v2 retraining

We ran two additional fits of the same v2 classifier to use more of the real Azure HTTP trace. The 0–30% function-hash fit used 2,321,712 training windows and calibrated on the disjoint 30–40% group. At the validation-selected 60% recall threshold, pooled days 13–14 scores on groups 50–100% were 46.61% precision and 63.22% recall. The original v2 had 45.56% precision and 63.46% recall, so this candidate traded recall for precision. Raising its validation recall target to 62% produced 44.65% precision and 65.05% recall on the same groups. Neither setting improved both measures.

The 0–40% fit used 3,256,248 training windows and calibrated on the disjoint 40–50% group. Its validation-selected 60% recall threshold was 0.2216713. The active artifact now contains that fit. Across the same 629,280 evaluation windows in groups 50–100%, pooled precision was **45.81%**, recall **64.23%**, and F1 **53.48%**, compared with **45.56%**, **63.46%**, and **53.04%** for the preceding v2. In the 70–80% group both precision and recall fell (49.2% to 48.1%, 61.4% to 60.6%). The previous v2 artifact and its report are retained in `model/artifacts/serverless_model_v2_original.joblib` and `model/artifacts/report_v2_original.json`.

These evaluation groups were inspected repeatedly while selecting features and training sizes. The gain is small and **not a fresh-holdout improvement claim**. The 2019 Azure trace alone has no overload or successful scaling labels. The separately supplied Huawei Public trace is evaluated below.

## Huawei Public 2023 external evaluation

The user supplied Huawei's `public_dataset.zip` in ignored `data/`. The archive passed its ZIP integrity check and contains all 26 daily request-count CSVs for 5,093 functions. Its schema has function IDs and per-minute request counts but **no HTTP-trigger column**, so this is an external FaaS workload evaluation rather than a direct HTTP-only comparison. The evaluator treats blank count cells as zero and applies the Azure eligibility rule (at least 300 daily requests and 60 active minutes). [The frozen-model report](artifacts/huawei_external_report.json) records the archive and artifact hashes, the same surge label, all 26 daily results, and pooled confusion counts.

The Azure-trained active model, without Huawei fitting or threshold changes, scored **39.32% precision, 62.93% recall, and 48.40% F1** on 1,561,332 Huawei windows. It produced 22,766 true alerts, 35,128 false alerts, and missed 13,410 surges. The no-surge baseline accuracy is 97.68%, above the model's 96.89% accuracy; accuracy is therefore especially unhelpful here.

We ran one separate Huawei-only classifier experiment with the same v2 architecture and past-only features. Days 0–15 trained 939,504 windows; days 16–20 calibrated probabilities and set a threshold targeting at least 65% validation recall; days 21–25 were reserved for comparison. On those final Huawei days, precision increased from **37.65% to 42.83%** and recall from **60.91% to 61.43%** versus the active model. On the previously inspected Azure evaluation groups, however, this candidate scored **43.59% precision and 52.95% recall**, below the active model's **45.81% and 64.23%**. The Huawei-only artifact was **not activated**. The final Huawei days were part of the earlier 26-day baseline report, so this experiment is exploratory rather than a pristine, unseen test.

The external result shows workload shift matters. A future model should be trained and calibrated on representative traffic from the actual Go demo application and evaluated on untouched later runs; if using multiple cloud traces, use explicit domain-aware validation and preserve a separate final holdout. Real scaling labels still require measured latency, failures, single-server capacity, and standby wake time.
