FROM golang:1.26-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY examples/billing ./examples/billing
RUN CGO_ENABLED=0 go build -trimpath -o /billing ./examples/billing

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /billing /billing
EXPOSE 8091
ENTRYPOINT ["/billing"]
