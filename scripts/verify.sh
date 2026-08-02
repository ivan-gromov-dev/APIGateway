#!/usr/bin/env sh

set -eu

SCRIPT_DIR=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
PROJECT_ROOT=$(CDPATH= cd -- "$SCRIPT_DIR/.." && pwd)
cd "$PROJECT_ROOT"

unformatted=$(
    find cmd examples internal test -type f -name '*.go' -exec sh -c '
        for file do
            current=$(tr -d "\r" <"$file")
            formatted=$(gofmt "$file")
            if [ "$current" != "$formatted" ]; then
                printf "%s\n" "$file"
            fi
        done
    ' sh {} +
)
if [ -n "$unformatted" ]; then
    echo "The following Go files require gofmt:" >&2
    echo "$unformatted" >&2
    exit 1
fi

go vet ./...

if [ "${RACE:-0}" = "1" ]; then
    go test -race -count=1 ./...
else
    go test -count=1 ./...
fi

mkdir -p .cache/verify
coverage_failed=0
for package in $(go list ./internal/...); do
    profile_name=$(printf '%s' "$package" | tr -c 'A-Za-z0-9_-' '_')
    profile=".cache/verify/coverage-${profile_name}.out"
    if [ "${RACE:-0}" = "1" ]; then
        go test -race -count=1 -covermode=atomic -coverprofile="$profile" "$package"
    else
        go test -count=1 -covermode=atomic -coverprofile="$profile" "$package"
    fi
    percentage=$(go tool cover -func="$profile" | awk '/^total:/ {gsub(/%/, "", $3); print $3}')
    printf '%-70s %6.1f%%\n' "$package" "$percentage"
    if ! awk -v coverage="$percentage" 'BEGIN { exit !(coverage > 75) }'; then
        echo "$package coverage $percentage% must be greater than 75%" >&2
        coverage_failed=1
    fi
done
if [ "$coverage_failed" -ne 0 ]; then
    exit 1
fi

go build -o .cache/verify/gateway ./cmd/gateway
go build -o .cache/verify/users ./examples/backend
go build -o .cache/verify/billing ./examples/billing
go build -o .cache/verify/identity ./examples/identity
go build -o .cache/verify/grpc-health ./examples/grpc-health

echo "Verification completed successfully."
