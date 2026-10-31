#!/usr/bin/env bash
set -euo pipefail
ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
cd "$ROOT/web"
npm ci
npm run build
cd "$ROOT"
mkdir -p bin
CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o bin/com-itemgather ./cmd/com-itemgather
printf 'Built web/dist and bin/com-itemgather\n'
