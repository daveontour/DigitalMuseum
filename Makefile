.PHONY: build build-exe build-launcher build-runner test generate lint run run-runner clean tidy

MODULE := github.com/daveontour/digitalmuseum
BINARY := digitalmuseum
CMD     := ./cmd/server
RUNNER  := ./cmd/runner

build:
	go build -o bin/$(BINARY) $(CMD)

build-exe:
	go build -o bin/$(BINARY).exe $(CMD)

build-launcher:
	go build -buildvcs=false -ldflags="-H windowsgui" -o launcher.exe ./cmd/launcher

build-runner:
	go build -o runner.exe $(RUNNER)

run:
	go run $(CMD)

run-runner:
	go run $(RUNNER) -config runner.json

test:
	go test ./...

test-verbose:
	go test -v ./...

generate:
	sqlc generate

lint:
	golangci-lint run ./...

tidy:
	go mod tidy

clean:
	rm -f bin/$(BINARY)

# Run with race detector
race:
	go run -race $(CMD)

# Build and run (convenience)
dev: build
	./bin/$(BINARY)
