#!/usr/bin/env sh
set -eu

base_url="${BASE_URL:-http://localhost:8080}"
admin_url="${ADMIN_URL:-http://localhost:9090}"
curl --fail --silent --show-error "$admin_url/healthz" >/dev/null
curl --fail --silent --show-error "$admin_url/readyz" >/dev/null
first_headers="$(mktemp)"
second_headers="$(mktemp)"
trap 'rm -f "$first_headers" "$second_headers"' EXIT
curl --fail --silent --show-error --dump-header "$first_headers" "$base_url/api/billing/invoices" >/dev/null
curl --fail --silent --show-error --dump-header "$second_headers" "$base_url/api/billing/invoices" >/dev/null
grep -qi '^x-cache: \(MISS\|HIT\)' "$first_headers"
grep -qi '^x-cache: HIT' "$second_headers"
token="$(curl --fail --silent --show-error -u gateway-demo:gateway-demo-secret -d grant_type=client_credentials -d scope=users.read http://localhost:8084/token | sed -n 's/.*"access_token":"\([^"]*\)".*/\1/p')"
test -n "$token"
curl --fail --silent --show-error -H "Authorization: Bearer $token" -H 'X-User-ID: smoke-user' "$base_url/api/users" >/dev/null
curl --fail --silent --show-error "$admin_url/metrics" | grep -q gateway_feature_decisions_total
go run ./examples/grpc-health -check localhost:8080
printf '%s\n' 'Docker smoke checks passed.'
