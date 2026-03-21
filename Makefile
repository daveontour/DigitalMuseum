.PHONY: build test generate lint run clean tidy

MODULE := github.com/daveontour/digitalmuseum
BINARY := digitalmuseum
CMD     := ./cmd/server

build:
	go build -o bin/$(BINARY) $(CMD)

build-exe:
	go build -o bin/$(BINARY).exe $(CMD)

build-launcher:
	go build -ldflags="-H windowsgui" -o launcher.exe ./cmd/launcher

run:
	go run $(CMD)

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
