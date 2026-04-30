#!/usr/bin/env bash
set -euo pipefail

cd "$(dirname "$0")/.."

echo "==> Starting postgres + redis..."
docker compose up -d --wait

echo "==> Running benchmark (all 3 strategies × 3 workloads)..."
go run ./cmd/benchmark/... "$@"
