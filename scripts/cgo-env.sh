#!/usr/bin/env bash
# Export CGO include paths for sqlite-vec and go-sqlite3.
# Usage (from repo root): source scripts/cgo-env.sh
# Or: source scripts/cgo-env.sh && go build -o bin/digitalmuseum.exe ./cmd/server

# GCC on Windows (MinGW) needs forward-slash paths like D:/repo/cgo-compat, not /d/repo/...
normalize_cgo_path() {
	local p="$1"
	if command -v cygpath >/dev/null 2>&1; then
		p="$(cygpath -m "$p")"
	fi
	printf '%s' "$p" | tr '\\' '/'
}

_script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT="$(normalize_cgo_path "$(cd "${_script_dir}/.." && pwd)")"

SQLITE_MOD=""
if command -v go >/dev/null 2>&1; then
	SQLITE_MOD="$(go list -f '{{.Dir}}' -m github.com/mattn/go-sqlite3 2>/dev/null || true)"
	if [[ -n "${SQLITE_MOD}" ]]; then
		SQLITE_MOD="$(normalize_cgo_path "${SQLITE_MOD}")"
	fi
fi

export CGO_ENABLED=1
_cgo_inc="-I${ROOT}/cgo-compat"
if [[ -n "${SQLITE_MOD}" ]]; then
	_cgo_inc="${_cgo_inc} -I${SQLITE_MOD}"
fi
export CGO_CFLAGS="${_cgo_inc} ${CGO_CFLAGS:-}"
