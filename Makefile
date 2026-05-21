.PHONY: check-go build build-exe build-exe-electron build-linux build-launcher test generate lint run clean tidy sqlitevec-demo sqlitevec-demo-seed message-search-cli

MODULE := github.com/daveontour/aimuseum
BINARY := digitalmuseum
CMD     := ./cmd/server

# SQLite uses github.com/mattn/go-sqlite3 (CGO). You need a C compiler on PATH
# (e.g. MSYS2 mingw-w64 gcc on Windows). CGO_ENABLED=1 is required.
#
# Prepend the directory that contains gcc so cgo-spawned cc1.exe resolves the
# same MinGW/MSYS2 DLLs (reduces opaque "cgo.exe: exit status 2" on Windows).
GCC := $(shell command -v gcc 2>/dev/null)
ifneq ($(GCC),)
GCCDIR := $(dir $(GCC))
ifneq ($(GCCDIR),./)
export PATH := $(GCCDIR):$(PATH)
endif
endif

# sqlite-vec-go-bindings/cgo includes "sqlite3.h". Always inject cgo-compat so a
# cold build works: until modules are fetched, `go list -m` has no Dir for go-sqlite3
# (first `go build` was downloading deps, so CGO previously had no sqlite3.h includes).
_SQLITE_RAW := $(shell go list -f '{{.Dir}}' -m github.com/mattn/go-sqlite3 2>/dev/null)
SQLITE3_BINDING_DIR := $(shell printf '%s' '$(_SQLITE_RAW)' | tr '\\' '/' 2>/dev/null || printf '%s' '$(_SQLITE_RAW)')
export CGO_CFLAGS := -I$(CURDIR)/cgo-compat $(if $(strip $(SQLITE3_BINDING_DIR)),-I$(SQLITE3_BINDING_DIR)) $(CGO_CFLAGS)

# Go 1.25+ with CGO on Windows can emit a PE that the loader rejects (“This app can't
# run on your PC”) when debug info is retained (malformed headers / DWARF interaction;
# see https://go.dev/issue/75121 ). Always strip for Windows server/launcher builds.
WINDOWS_STRIP_LDF := $(if $(filter windows,$(shell go env GOOS 2>/dev/null)),-s -w,)

# go.mod is 1.25+; grpc and stdlib (log/slog, slices, …) need a current toolchain.
check-go:
	@go version >/dev/null 2>&1 || { echo >&2 "go: not found in PATH"; exit 1; }
	@if go version | grep -qE ' go1\.([0-9]\.|1[0-9]\.|2[0-4]\.)'; then \
		echo >&2 "This project requires Go 1.25 or newer."; \
		echo >&2 "Current: $$(go version)"; \
		echo >&2 "Install: winget upgrade GoLang.Go   or   https://go.dev/dl/"; \
		exit 1; \
	fi

build: check-go
	go build $(if $(WINDOWS_STRIP_LDF),-ldflags="$(WINDOWS_STRIP_LDF)") -o bin/$(BINARY) $(CMD)

build-exe: check-go
	go build $(if $(WINDOWS_STRIP_LDF),-ldflags="$(WINDOWS_STRIP_LDF)") -o bin/$(BINARY).exe $(CMD)

# Windowsgui subsystem build — no console window when launched by Electron.
build-exe-electron: check-go
	go build -ldflags="$(strip $(WINDOWS_STRIP_LDF) -H windowsgui)" -o bin/$(BINARY).exe $(CMD)

build-linux: check-go
	@hostos="$$(go env GOOS)"; \
	if [ "$$hostos" != "linux" ]; then \
		echo >&2 "build-linux: cannot cross-compile to linux/amd64 from $$hostos (go-sqlite3 requires CGO + a Linux C toolchain)."; \
		echo >&2 "Run this target on Linux amd64, or build inside a Linux container/CI job."; \
		exit 1; \
	fi
	GOOS=linux GOARCH=amd64 CGO_ENABLED=1 go build -o bin/$(BINARY)-linux-amd64 $(CMD)

build-launcher: check-go
	go build -buildvcs=false -ldflags="$(strip $(WINDOWS_STRIP_LDF) -H windowsgui)" -o launcher.exe ./cmd/launcher

run: build-exe
	./bin/$(BINARY).exe

test: check-go
	go test ./...

test-verbose: check-go
	go test -v ./...

generate:
	sqlc generate

lint:
	golangci-lint run ./...

tidy: check-go
	go mod tidy

clean:
	rm -f bin/$(BINARY)

# Run with race detector
race: check-go
	go run -race $(CMD)

# Build and run (convenience)
dev: build
	./bin/$(BINARY)

# ── Electron targets ──────────────────────────────────────────────────────────

# Run Electron in dev mode against the local source tree (no packaging).
# Uses the console-subsystem binary (plain `go build`); spawning a WINDOWSGUI exe
# with piped stdout/stderr can fail on some Windows setups with spawn UNKNOWN (-4094).
# Packaged installers still use build-exe-electron (hidden console).
electron-dev: build-exe
	cd electron && npm install --prefer-offline && npx electron .

# Package the Electron app into a distributable installer.
# Produces dist/electron/Digital Museum Setup *.exe (config: electron/electron-builder.yml).
# Cleans dist/electron first so a stale or locked *.nsis.7z does not break NSIS (mmap errors on Windows).
electron-dist: build-exe-electron
	@test -f bin/$(BINARY).exe || { echo >&2 "Missing bin/$(BINARY).exe — run from repo root after build-exe-electron."; exit 1; }
	rm -rf dist/electron
	cd electron && npm install --prefer-offline && npx electron-builder

# sqlite-vec demo CLI. Usage:
# make sqlitevec-demo QUERY="family photos"
sqlitevec-demo: check-go
	cd cmd/sqlitevec-demo && go run . -query "$(if $(QUERY),$(QUERY),family memories and photos)"

# Seed the demo DB without running a search query.
sqlitevec-demo-seed: check-go
	cd cmd/sqlitevec-demo && go run . -seed-only

# Message similarity CLI. Usage:
# make message-search-cli QUERY="text to search" N=5
message-search-cli: check-go
	go run ./cmd/message-search-cli -q "$(if $(QUERY),$(QUERY),family and photos)" -n "$(if $(N),$(N),5)"
