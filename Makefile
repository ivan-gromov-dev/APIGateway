.PHONY: build run test test-unit test-integration verify lint tidy

build:
	go build -trimpath -o bin/gateway ./cmd/gateway

run:
	go run ./cmd/gateway -config configs/gateway.yaml

test:
	go test -race -cover ./...

test-unit:
	go test -race -count=1 ./internal/...

test-integration:
	go test -race -count=1 ./test/integration/...

verify:
	sh scripts/verify.sh

lint:
	go vet ./...

tidy:
	go mod tidy
