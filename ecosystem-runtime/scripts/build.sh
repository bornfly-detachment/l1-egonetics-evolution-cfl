#!/usr/bin/env bash
set -euo pipefail
out="${1:-./bin/evolutionctl}"
mkdir -p "$(dirname "$out")"
go build -o "$out" ./cmd/evolutionctl
printf '%s\n' "$out"
