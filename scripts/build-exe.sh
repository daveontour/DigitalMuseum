#!/usr/bin/env bash
# Build bin/digitalmuseum.exe with the same CGO flags as the Makefile (console subsystem).
set -euo pipefail
ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "${ROOT}"
# shellcheck source=scripts/cgo-env.sh
source "${ROOT}/scripts/cgo-env.sh"

LDFLAGS="-s -w"
if [[ "$(go env GOOS)" == "windows" ]]; then
	LDFLAGS="${LDFLAGS}"
fi

mkdir -p bin
go build -ldflags="${LDFLAGS}" -o bin/digitalmuseum.exe ./cmd/server
echo "Built ${ROOT}/bin/digitalmuseum.exe"
