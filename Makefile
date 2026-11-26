VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
COMMIT  ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo none)
DATE    ?= $(shell date -u +%Y-%m-%dT%H:%M:%SZ)
LDFLAGS := -s -w -X main.version=$(VERSION) -X main.commit=$(COMMIT) -X main.date=$(DATE)

PLATFORMS := linux/amd64 linux/arm64 darwin/amd64 darwin/arm64 windows/amd64 windows/arm64

.PHONY: build run dist snapshot test cover vet lint fix format tidy check clean

build:
	go build -trimpath -ldflags "$(LDFLAGS)" -o dist/cln$(if $(filter Windows_NT,$(OS)),.exe,) ./cmd/cln

run:
	go run ./cmd/cln $(ARGS)

dist:
	@for p in $(PLATFORMS); do \
		os=$${p%/*}; arch=$${p#*/}; ext=; [ $$os = windows ] && ext=.exe; \
		echo "building $$os/$$arch"; \
		CGO_ENABLED=0 GOOS=$$os GOARCH=$$arch go build -trimpath -ldflags "$(LDFLAGS)" \
			-o dist/cln-$$os-$$arch$$ext ./cmd/cln || exit 1; \
	done

snapshot:
	goreleaser release --snapshot --clean

test:
	go test -race ./...

cover:
	go test -coverprofile=coverage.out ./...
	go tool cover -func=coverage.out

vet:
	go vet ./...

lint:
	golangci-lint run ./...

fix:
	golangci-lint run --fix ./...

format:
	golangci-lint fmt ./...

tidy:
	go mod tidy

check: tidy vet lint test

clean:
	rm -rf dist
