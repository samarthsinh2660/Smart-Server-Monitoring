#!/usr/bin/env bash
set -euo pipefail
cd -- "$(dirname -- "${BASH_SOURCE[0]}")"
mkdir -p bin/windows-amd64
for service in agent demoapp; do
  CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build -o "bin/windows-amd64/${service}.exe" "./cmd/${service}"
done
CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build -o bin/windows-amd64/traffic-lab.exe ./demo/traffic-lab
# Compile native checks for the user to run after booting Windows; do not run on Linux.
CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go test -c -o bin/windows-amd64/metrics.test.exe ./internal/metrics
echo 'Windows executables written to bin/windows-amd64/'
