#!/usr/bin/env bash
set -euo pipefail
cd -- "$(dirname -- "${BASH_SOURCE[0]}")"
if [[ -f config/pi.env ]]; then
  set -a
  source config/pi.env
  set +a
fi
: "${APP_URL:?Set APP_URL to the primary laptop LAN origin}"
: "${AGENT_URL:?Set AGENT_URL to the primary laptop metrics origin}"
export CONTROLLER_ADDR="${CONTROLLER_ADDR:-0.0.0.0:8080}"
export OMP_NUM_THREADS="${OMP_NUM_THREADS:-1}"
export OPENBLAS_NUM_THREADS="${OPENBLAS_NUM_THREADS:-1}"
mkdir -p bin
CGO_ENABLED=1 go build -o bin/controller ./cmd/controller
model_python="${MODEL_PYTHON:-python3}"
if [[ -z "${MODEL_PYTHON:-}" && -x .venv/bin/python ]]; then model_python=.venv/bin/python; fi
model_artifact="${MODEL_ARTIFACT:-model/artifacts/serverless_model.joblib}"
model_ready=false
if [[ -f "$model_artifact" ]] && "$model_python" -c 'import joblib, numpy, scipy, sklearn'; then
  model_ready=true
else
  echo 'Prediction unavailable: model artifact or Python dependencies missing. The controller and CPU activation will still run.' >&2
fi
model_args=(--artifact "$model_artifact" --controller-url "${MODEL_CONTROLLER_URL:-http://127.0.0.1:8080}")
if [[ -n "${MODEL_CAPACITY_RPM:-}" ]]; then model_args+=(--capacity-rpm "$MODEL_CAPACITY_RPM"); fi
if [[ "${AI_SCALING_VALIDATED:-false}" == true ]]; then
  if [[ -n "${MODEL_CAPACITY_RPM:-}" ]]; then
    model_args+=(--enable-operational-scaling)
  else
    echo 'Operational AI was requested without measured MODEL_CAPACITY_RPM; inference stays in shadow mode.' >&2
  fi
fi
pids=()
cleanup() {
  trap - EXIT INT TERM
  for pid in "${pids[@]}"; do kill "$pid" 2>/dev/null || true; done
  for pid in "${pids[@]}"; do wait "$pid" 2>/dev/null || true; done
}
trap cleanup EXIT
trap 'exit 130' INT
trap 'exit 143' TERM
./bin/controller &
controller_pid=$!
pids+=("$controller_pid")
if [[ "$model_ready" == true ]]; then
  "$model_python" -m model.serve "${model_args[@]}" &
  pids+=("$!")
fi
echo 'Pi controller started; inference is optional. HAProxy must be running separately. Ctrl+C stops these processes.'
# Model failure must not stop monitoring, CPU activation, or app-health failover.
if wait "$controller_pid"; then echo 'Pi controller stopped.' >&2; else echo 'Pi controller failed.' >&2; fi
exit 1
