.PHONY: build run test lint tidy

build:
	go build -trimpath -o bin/gateway ./cmd/gateway

run:
	go run ./cmd/gateway -config configs/gateway.yaml

test:
	go test -race -cover ./...

lint:
	go vet ./...

tidy:
	go mod tidy
