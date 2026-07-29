FROM golang:1.26-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /gateway ./cmd/gateway

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /gateway /gateway
COPY configs/gateway.docker.yaml /configs/gateway.yaml
EXPOSE 8080 9090
ENTRYPOINT ["/gateway", "-config", "/configs/gateway.yaml"]
