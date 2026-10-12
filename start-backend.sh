#!/usr/bin/env bash
set -euo pipefail

cd -- "$(dirname -- "${BASH_SOURCE[0]}")"

mkdir -p bin
go build -o bin/agent ./cmd/agent
go build -o bin/demoapp ./cmd/demoapp
CGO_ENABLED=1 go build -o bin/controller ./cmd/controller

pids=()
cleanup() {
  trap - EXIT INT TERM
  for pid in "${pids[@]}"; do
    kill "$pid" 2>/dev/null || true
  done
  for pid in "${pids[@]}"; do
    wait "$pid" 2>/dev/null || true
  done
}
trap cleanup EXIT
trap 'exit 130' INT
trap 'exit 143' TERM

./bin/agent &
pids+=("$!")
./bin/demoapp &
pids+=("$!")
./bin/controller &
pids+=("$!")

echo "Agent, demo app, and controller started. Press Ctrl+C to stop all three."
if wait -n "${pids[@]}"; then
  echo "A backend service stopped; stopping the others."
else
  echo "A backend service failed; stopping the others." >&2
fi
exit 1
