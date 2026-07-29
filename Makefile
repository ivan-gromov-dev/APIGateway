.PHONY: build run test test-integration lint tidy

build:
	go build -trimpath -o bin/gateway ./cmd/gateway

run:
	go run ./cmd/gateway -config configs/gateway.yaml

test:
	go test -race -cover ./...

test-integration:
	go test -race -count=1 ./test/integration/...

lint:
	go vet ./...

tidy:
	go mod tidy
