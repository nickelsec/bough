# Everything here also runs in CI. If a target passes locally and fails there,
# the difference is worth chasing rather than working around.

BINARY := bough

.PHONY: all
all: lint test build

.PHONY: build
build:
	go build -o $(BINARY) ./cmd/bough

.PHONY: test
test:
	go test ./...

# The race detector needs cgo. The shipped binary is pure Go and does not.
.PHONY: race
race:
	CGO_ENABLED=1 go test -race ./...

.PHONY: lint
lint:
	gofmt -l .
	go vet ./...
	golangci-lint run ./...

.PHONY: cover
cover:
	go test -coverprofile=coverage.out ./...
	go tool cover -func=coverage.out | tail -1

# A minute is enough to catch a regression in the parser, which is the one
# place a malformed record could take the whole thing down.
.PHONY: fuzz
fuzz:
	go test -run=xxx -fuzz=FuzzReadRecords -fuzztime=60s ./internal/agent/claude

# Regenerates the embedded artwork and the subset fonts. Needs Python and
# fontTools, which building does not.
.PHONY: assets
assets:
	python assets/build.py
	python assets/fonts.py

.PHONY: clean
clean:
	rm -f $(BINARY) $(BINARY).exe coverage.out
