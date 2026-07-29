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
go build -o .cache/verify/gateway ./cmd/gateway
go build -o .cache/verify/users ./examples/backend
go build -o .cache/verify/billing ./examples/billing

echo "Verification completed successfully."
