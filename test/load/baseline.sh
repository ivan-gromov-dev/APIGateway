#!/usr/bin/env sh

set -eu

DURATION="60s"
CONCURRENCY=100
WARMUP_REQUESTS=1000
WARMUP_CONCURRENCY=20
GATEWAY_URL="http://localhost:8080"
ADMIN_URL="http://localhost:9090"

usage() {
    cat <<'EOF'
Usage: sh test/load/baseline.sh [options]

Options:
  --duration VALUE             Scenario duration (default: 60s)
  --concurrency VALUE          Read concurrency (default: 100)
  --warmup-requests VALUE      Warm-up request count (default: 1000)
  --warmup-concurrency VALUE   Warm-up concurrency (default: 20)
  --gateway-url URL            Gateway base URL
  --admin-url URL              Admin server base URL
  -h, --help                   Show this help
EOF
}

require_value() {
    if [ "$#" -lt 2 ]; then
        echo "Missing value for $1" >&2
        usage >&2
        exit 2
    fi
}

require_positive_integer() {
    name=$1
    value=$2
    case "$value" in
        ''|*[!0-9]*|0)
            echo "$name must be a positive integer, got: $value" >&2
            exit 2
            ;;
    esac
}

while [ "$#" -gt 0 ]; do
    case "$1" in
        --duration)
            require_value "$@"
            DURATION=$2
            shift 2
            ;;
        --concurrency)
            require_value "$@"
            CONCURRENCY=$2
            shift 2
            ;;
        --warmup-requests)
            require_value "$@"
            WARMUP_REQUESTS=$2
            shift 2
            ;;
        --warmup-concurrency)
            require_value "$@"
            WARMUP_CONCURRENCY=$2
            shift 2
            ;;
        --gateway-url)
            require_value "$@"
            GATEWAY_URL=$2
            shift 2
            ;;
        --admin-url)
            require_value "$@"
            ADMIN_URL=$2
            shift 2
            ;;
        -h|--help)
            usage
            exit 0
            ;;
        *)
            echo "Unknown option: $1" >&2
            usage >&2
            exit 2
            ;;
    esac
done

require_positive_integer "concurrency" "$CONCURRENCY"
require_positive_integer "warmup requests" "$WARMUP_REQUESTS"
require_positive_integer "warmup concurrency" "$WARMUP_CONCURRENCY"

if ! command -v hey >/dev/null 2>&1; then
    cat >&2 <<'EOF'
hey was not found.
Install it with:
  go install github.com/rakyll/hey@latest
EOF
    exit 1
fi

if ! command -v curl >/dev/null 2>&1; then
    echo "curl is required to check gateway readiness." >&2
    exit 1
fi

GATEWAY_URL=${GATEWAY_URL%/}
ADMIN_URL=${ADMIN_URL%/}
READY_URL="$ADMIN_URL/readyz"

ready_response=$(curl --fail --silent --show-error --max-time 5 "$READY_URL") || {
    echo "Gateway is not ready at $READY_URL." >&2
    echo "Start it with: docker compose up --build -d" >&2
    exit 1
}

case "$ready_response" in
    *'"status":"ready"'*) ;;
    *)
        echo "Unexpected readiness response: $ready_response" >&2
        exit 1
        ;;
esac

SCRIPT_DIR=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
RESULTS_DIRECTORY="$SCRIPT_DIR/results"
mkdir -p "$RESULTS_DIRECTORY"

TIMESTAMP=$(date '+%Y%m%d-%H%M%S')
RESULT_FILE="$RESULTS_DIRECTORY/baseline-$TIMESTAMP.txt"
WRITE_CONCURRENCY=$((CONCURRENCY / 2))
if [ "$WRITE_CONCURRENCY" -lt 1 ]; then
    WRITE_CONCURRENCY=1
fi

cat >"$RESULT_FILE" <<EOF
API Gateway performance baseline
Created:             $(date '+%Y-%m-%d %H:%M:%S %z')
Gateway:             $GATEWAY_URL
Duration:            $DURATION
Read concurrency:    $CONCURRENCY
Write concurrency:   $WRITE_CONCURRENCY
Warm-up requests:    $WARMUP_REQUESTS
Warm-up concurrency: $WARMUP_CONCURRENCY
hey executable:      $(command -v hey)
EOF

run_scenario() {
    scenario_name=$1
    shift

    separator="================================================================================"
    {
        echo
        echo "$separator"
        echo "Scenario: $scenario_name"
        echo "Started:  $(date '+%Y-%m-%d %H:%M:%S %z')"
        echo "Command:  hey $*"
        echo "$separator"
    } | tee -a "$RESULT_FILE"

    set +e
    scenario_output=$(hey "$@" 2>&1)
    scenario_status=$?
    set -e

    printf '%s\n' "$scenario_output" | tee -a "$RESULT_FILE"
    if [ "$scenario_status" -ne 0 ]; then
        echo "hey failed for scenario '$scenario_name' with exit code $scenario_status" >&2
        exit "$scenario_status"
    fi
}

echo "Warming up the gateway..."
if ! hey -n "$WARMUP_REQUESTS" -c "$WARMUP_CONCURRENCY" \
    "$GATEWAY_URL/api/users" >/dev/null 2>&1; then
    echo "Gateway warm-up failed." >&2
    exit 1
fi

run_scenario "Users GET" \
    -z "$DURATION" \
    -c "$CONCURRENCY" \
    "$GATEWAY_URL/api/users"

run_scenario "Billing GET" \
    -z "$DURATION" \
    -c "$CONCURRENCY" \
    "$GATEWAY_URL/api/billing/invoices"

run_scenario "Billing POST" \
    -z "$DURATION" \
    -c "$WRITE_CONCURRENCY" \
    -m POST \
    -H "Content-Type: application/json" \
    -d "{}" \
    "$GATEWAY_URL/api/billing/payments"

echo
echo "Baseline completed."
echo "Results: $RESULT_FILE"
