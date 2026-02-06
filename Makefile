BINARY  := golink
MODULE  := github.com/romainlancelot/golink
MAIN    := ./cmd/golink

# Raspberry Pi (ARMv6) cross-compilation defaults
GOOS    ?= linux
GOARCH  ?= arm
GOARM   ?= 6

LDFLAGS := -s -w

.PHONY: all build build-local run clean fmt vet lint test deploy help

all: build

## build: Cross-compile for Raspberry Pi (ARMv6)
build:
	GOOS=$(GOOS) GOARCH=$(GOARCH) GOARM=$(GOARM) go build -ldflags="$(LDFLAGS)" -o $(BINARY) $(MAIN)
	@echo "Built $(BINARY) for $(GOOS)/$(GOARCH) (ARM v$(GOARM))"

## build-local: Build for the current platform
build-local:
	go build -ldflags="$(LDFLAGS)" -o $(BINARY) $(MAIN)

## run: Build and run locally
run: build-local
	./$(BINARY)

## fmt: Format all Go source files
fmt:
	gofmt -s -w .

## vet: Run go vet
vet:
	go vet ./...

## lint: Run staticcheck (install: go install honnef.co/go/tools/cmd/staticcheck@latest)
lint: vet
	staticcheck ./...

## test: Run tests
test:
	go test -race ./...

## clean: Remove build artifacts
clean:
	rm -f $(BINARY)

## deploy: Copy binary and service file to Raspberry Pi (set PI_HOST)
deploy: build
	@[ "$(PI_HOST)" ] || { echo "Set PI_HOST (e.g. make deploy PI_HOST=pi@192.168.1.10)"; exit 1; }
	scp $(BINARY) deploy/golink.service $(PI_HOST):/tmp/
	@echo "Files copied. SSH into the Pi and run:"
	@echo "  sudo mv /tmp/golink /usr/local/bin/"
	@echo "  sudo mv /tmp/golink.service /etc/systemd/system/"
	@echo "  sudo systemctl daemon-reload && sudo systemctl restart golink"

## help: Show available targets
help:
	@grep -E '^## ' Makefile | sed 's/## //' | column -t -s ':'
