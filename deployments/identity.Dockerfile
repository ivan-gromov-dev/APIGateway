FROM golang:1.26-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY examples/identity ./examples/identity
RUN CGO_ENABLED=0 go build -trimpath -o /identity ./examples/identity

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /identity /identity
EXPOSE 8084
ENTRYPOINT ["/identity"]
