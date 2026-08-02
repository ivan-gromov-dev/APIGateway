FROM golang:1.26-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY examples/grpc-health ./examples/grpc-health
RUN CGO_ENABLED=0 go build -trimpath -o /grpc-health ./examples/grpc-health

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /grpc-health /grpc-health
EXPOSE 50051
ENTRYPOINT ["/grpc-health"]
