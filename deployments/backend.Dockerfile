FROM golang:1.26-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY examples/backend ./examples/backend
RUN CGO_ENABLED=0 go build -trimpath -o /backend ./examples/backend

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /backend /backend
EXPOSE 8081
ENTRYPOINT ["/backend"]
