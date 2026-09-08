#!/usr/bin/env bash
# profile.sh — Run a simulation scenario and produce a comprehensive
# performance report with per-endpoint latency, SQL query stats, and
# optional pprof CPU/heap profiles.
#
# Replaces the earlier sql_profile.sh with:
#   - Request latency percentiles (from logging middleware's duration_ms)
#   - Read/write traffic support (--read-traffic, --read-multiplier)
#   - Concurrent user simulation (--concurrency)
#   - pprof CPU + heap capture (--pprof)
#   - Comparison mode (--compare <previous-report.md>)
#
# Prerequisites:
#   - Local PostgreSQL running (npm run db:start)
#   - Generated code exists (npm run generate)
#   - jq installed
#
# Usage:
#   ./scripts/profile.sh                                          # defaults
#   ./scripts/profile.sh --scenario load-test-small -o report.md  # with output
#   ./scripts/profile.sh --concurrency 5 --read-traffic           # concurrent + reads
#   ./scripts/profile.sh --pprof /tmp/profiles                    # capture pprof
#   ./scripts/profile.sh --compare docs/ai/profile_baseline.md    # show deltas

set -euo pipefail
cd "$(git rev-parse --show-toplevel)"

# --- Default configuration ---------------------------------------------------
SCENARIO="load-test-small"
SEED=42
CONCURRENCY=0
READ_TRAFFIC=""
READ_MULTIPLIER=""
OUTPUT_FILE=""
PPROF_DIR=""
COMPARE_FILE=""
VERBOSE=""
SERVER_PORT=8080

# --- Parse flags --------------------------------------------------------------
while [[ $# -gt 0 ]]; do
  case $1 in
    --scenario)      SCENARIO="$2"; shift 2 ;;
    --seed)          SEED="$2"; shift 2 ;;
    --concurrency)   CONCURRENCY="$2"; shift 2 ;;
    --read-traffic)  READ_TRAFFIC="1"; shift ;;
    --read-multiplier) READ_MULTIPLIER="$2"; shift 2 ;;
    -o|--output)     OUTPUT_FILE="$2"; shift 2 ;;
    --pprof)         PPROF_DIR="$2"; shift 2 ;;
    --compare)       COMPARE_FILE="$2"; shift 2 ;;
    -v|--verbose)    VERBOSE="1"; shift ;;
    --port)          SERVER_PORT="$2"; shift 2 ;;
    -h|--help)
      echo "Usage: $0 [options]"
      echo ""
      echo "Options:"
      echo "  --scenario NAME       Simulation scenario (default: load-test-small)"
      echo "  --seed N              PRNG seed (default: 42)"
      echo "  --concurrency N       Concurrent user goroutines (default: 0 = sequential)"
      echo "  --read-traffic        Enable read actions (feed, search, browse)"
      echo "  --read-multiplier N   Scale read frequency (default: 1.0)"
      echo "  -o, --output FILE     Write markdown report to FILE"
      echo "  --pprof DIR           Save pprof CPU + heap profiles to DIR"
      echo "  --compare FILE        Compare results against a previous report"
      echo "  --port PORT           Server port (default: 8080)"
      echo "  -v, --verbose         Enable debug logging"
      echo "  -h, --help            Show this help"
      exit 0
      ;;
    *) echo "Unknown option: $1" >&2; exit 1 ;;
  esac
done

# --- Check prerequisites -----------------------------------------------------
if ! command -v jq &>/dev/null; then
  echo "Error: jq is required. Install with: brew install jq" >&2
  exit 1
fi

SERVER_URL="http://localhost:$SERVER_PORT"
SERVER_LOG="/tmp/profile-server.log"
STATS_FILE="/tmp/profile-stats.jsonl"
LATENCY_FILE="/tmp/profile-latency.jsonl"
COMMIT_HASH=$(git rev-parse --short HEAD)
BRANCH=$(git branch --show-current)
TIMESTAMP=$(date -u +"%Y-%m-%dT%H:%M:%SZ")

echo "=== Performance Profile ==="
echo "Branch: $BRANCH  Commit: $COMMIT_HASH"
echo "Scenario: $SCENARIO  Seed: $SEED  Concurrency: $CONCURRENCY"
if [ -n "$READ_TRAFFIC" ]; then
  echo "Read traffic: enabled (multiplier: ${READ_MULTIPLIER:-1.0})"
fi
echo ""

# --- Build server -------------------------------------------------------------
echo "Building server..."
mkdir -p tmp
go build -o tmp/server ./server

# --- Start server -------------------------------------------------------------
echo "Starting server (log: $SERVER_LOG)..."
LOG_LEVEL="info"
if [ -n "$VERBOSE" ]; then
  LOG_LEVEL="debug"
fi

# JWT signing secret comes from Secret Manager (dev project) via the
# developer's gcloud ADC. Notification provider is `noop` so we don't
# need the other server-side credentials for profiling.
JWT_SECRET=$(./scripts/fetch_secret.sh dev jwt-signing-secret)

./tmp/server \
  --log-level="$LOG_LEVEL" \
  --log-format=json \
  --dev-mode \
  --port="$SERVER_PORT" \
  --db=postgres://ripls:ripls_dev@localhost:5432/ripls?sslmode=disable \
  --local-media-storage=./media_storage \
  --notification-provider=noop \
  --invite-link-hostname=localhost:$SERVER_PORT \
  --embedding-model-path=./model_tuning/ripls_embedding.onnx \
  --embedding-vocab-path=./model_tuning/ripls_embedding_tokenizer/vocab.txt \
  --jwt-signing-secret="$JWT_SECRET" \
  > "$SERVER_LOG" 2>&1 &
SERVER_PID=$!

cleanup() {
  if kill -0 "$SERVER_PID" 2>/dev/null; then
    kill "$SERVER_PID" 2>/dev/null || true
    wait "$SERVER_PID" 2>/dev/null || true
  fi
}
trap cleanup EXIT

# Wait for server to be ready.
echo -n "Waiting for server"
for i in $(seq 1 30); do
  if curl -sf "$SERVER_URL/health" > /dev/null 2>&1; then
    echo " ready!"
    break
  fi
  echo -n "."
  sleep 1
  if [ "$i" -eq 30 ]; then
    echo ""
    echo "Error: server did not become ready in 30s. Check $SERVER_LOG" >&2
    exit 1
  fi
done

# --- Start pprof capture (optional) ------------------------------------------
PPROF_PID=""
CPU_PROFILE=""
HEAP_PROFILE=""
if [ -n "$PPROF_DIR" ]; then
  mkdir -p "$PPROF_DIR"
  CPU_PROFILE="$PPROF_DIR/cpu.prof"
  HEAP_PROFILE="$PPROF_DIR/heap.prof"
  echo "Starting pprof CPU capture (will run for duration of simulation)..."
  # Start CPU profile capture in background — it runs until the curl completes
  # or we kill it. We use a long duration and kill it after simulation ends.
  curl -so "$CPU_PROFILE" "$SERVER_URL/debug/pprof/profile?seconds=600" &
  PPROF_PID=$!
fi

# --- Purge previous simulation data ------------------------------------------
echo "Purging previous simulation data..."
go run ./server/cmd/simulate -url "$SERVER_URL" -purge 2>&1 || true

# --- Run simulation -----------------------------------------------------------
SIM_START=$(date +%s)

SIM_ARGS=(-url "$SERVER_URL" -assets-dir server/simulation/assets -scenario "$SCENARIO" -seed "$SEED")
if [ "$CONCURRENCY" -gt 0 ]; then
  SIM_ARGS+=(-concurrency "$CONCURRENCY")
fi
if [ -n "$READ_TRAFFIC" ]; then
  SIM_ARGS+=(-read-traffic)
fi
if [ -n "$READ_MULTIPLIER" ]; then
  SIM_ARGS+=(-read-multiplier "$READ_MULTIPLIER")
fi

echo "Running simulation..."
echo "  go run ./server/cmd/simulate ${SIM_ARGS[*]}"
echo ""
go run ./server/cmd/simulate "${SIM_ARGS[@]}"

SIM_END=$(date +%s)
SIM_DURATION=$((SIM_END - SIM_START))
echo ""
echo "Simulation completed in ${SIM_DURATION}s."

# --- Stop pprof capture ------------------------------------------------------
if [ -n "$PPROF_PID" ]; then
  # Kill the background CPU profile curl — it will save whatever it captured.
  kill "$PPROF_PID" 2>/dev/null || true
  wait "$PPROF_PID" 2>/dev/null || true
  # Capture heap profile snapshot.
  echo "Capturing heap profile..."
  curl -so "$HEAP_PROFILE" "$SERVER_URL/debug/pprof/heap" 2>/dev/null || true
  echo "pprof profiles saved to $PPROF_DIR/"
fi

# --- Stop server to flush logs ------------------------------------------------
kill "$SERVER_PID" 2>/dev/null || true
wait "$SERVER_PID" 2>/dev/null || true
sleep 1

# --- Extract DB query stats ---------------------------------------------------
echo "Extracting stats from logs..."

# DB query stats: {"message":"request db stats","db_queries":N,"db_duration_ms":M,"path":"/..."}
grep '"db_queries"' "$SERVER_LOG" | jq -c '{path: .path, queries: .db_queries, db_ms: .db_duration_ms}' > "$STATS_FILE" 2>/dev/null || true

# Request latency stats: {"message":"http request","path":"/...","duration_ms":N,"status":S,"rpc_method":"Svc/Method","response_bytes":N}
grep '"http request"' "$SERVER_LOG" | jq -c '{
  path: .path,
  rpc_method: (.rpc_method // ""),
  status: .status,
  duration_ms: .duration_ms,
  response_bytes: (.response_bytes // 0)
}' > "$LATENCY_FILE" 2>/dev/null || true

TOTAL_REQUESTS=$(wc -l < "$LATENCY_FILE" | tr -d ' ')
TOTAL_DB_REQUESTS=$(wc -l < "$STATS_FILE" | tr -d ' ')
TOTAL_QUERIES=$(jq -s '[.[].queries] | add // 0' "$STATS_FILE")
TOTAL_DB_MS=$(jq -s '[.[].db_ms] | add // 0' "$STATS_FILE")
ERROR_COUNT=$(jq -s '[.[] | select(.status >= 400)] | length' "$LATENCY_FILE")

echo ""
echo "=== Summary ==="
echo "Total HTTP requests:    $TOTAL_REQUESTS"
echo "Requests with DB:       $TOTAL_DB_REQUESTS"
echo "Total SQL queries:      $TOTAL_QUERIES"
echo "Total DB time:          ${TOTAL_DB_MS}ms"
echo "Errors (4xx+5xx):       $ERROR_COUNT"
echo "Wall-clock duration:    ${SIM_DURATION}s"
echo ""

# --- Per-endpoint latency stats -----------------------------------------------
echo "=== Per-Endpoint Latency (RPC methods only) ==="
echo ""
printf "%-40s %6s %7s %7s %7s %7s %8s\n" "RPC METHOD" "CALLS" "P50ms" "P95ms" "P99ms" "MAXms" "ERR"
printf "%-40s %6s %7s %7s %7s %7s %8s\n" "----------" "-----" "-----" "-----" "-----" "-----" "---"

jq -sr '
  [.[] | select(.rpc_method != "")] |
  group_by(.rpc_method) |
  map({
    method: .[0].rpc_method,
    calls: length,
    errors: ([.[] | select(.status >= 400)] | length),
    p50: (sort_by(.duration_ms) | .[length * 0.5 | floor].duration_ms),
    p95: (sort_by(.duration_ms) | .[([length * 0.95 | floor, length - 1] | min)].duration_ms),
    p99: (sort_by(.duration_ms) | .[([length * 0.99 | floor, length - 1] | min)].duration_ms),
    max: ([.[].duration_ms] | max)
  }) |
  sort_by(-.calls) |
  .[] |
  [.method, .calls, .p50, .p95, .p99, .max, .errors] |
  @tsv
' "$LATENCY_FILE" | while IFS=$'\t' read -r method calls p50 p95 p99 max errors; do
  printf "%-40s %6s %7s %7s %7s %7s %8s\n" "$method" "$calls" "$p50" "$p95" "$p99" "$max" "$errors"
done

echo ""

# --- Per-endpoint DB query stats ----------------------------------------------
echo "=== Per-Endpoint DB Queries ==="
echo ""
printf "%-60s %6s %8s %5s %5s %5s %5s %5s\n" "ENDPOINT" "CALLS" "TOT_Q" "MIN" "AVG" "P50" "P95" "MAX"
printf "%-60s %6s %8s %5s %5s %5s %5s %5s\n" "--------" "-----" "-----" "---" "---" "---" "---" "---"

jq -sr '
  group_by(.path) |
  map({
    path: .[0].path,
    calls: length,
    total: ([.[].queries] | add),
    min: ([.[].queries] | min),
    max: ([.[].queries] | max),
    avg: (([.[].queries] | add) / length | . * 10 | round / 10),
    p50: (sort_by(.queries) | .[length * 0.5 | floor].queries),
    p95: (sort_by(.queries) | .[([length * 0.95 | floor, length - 1] | min)].queries)
  }) |
  sort_by(-.total) |
  .[] |
  [.path, .calls, .total, .min, .avg, .p50, .p95, .max] |
  @tsv
' "$STATS_FILE" | while IFS=$'\t' read -r path calls total min avg p50 p95 max; do
  printf "%-60s %6s %8s %5s %5s %5s %5s %5s\n" "$path" "$calls" "$total" "$min" "$avg" "$p50" "$p95" "$max"
done

echo ""

# --- Top offenders (latency x volume) ----------------------------------------
echo "=== Top 10 Latency Offenders (p95 * calls) ==="
echo ""

jq -sr '
  [.[] | select(.rpc_method != "")] |
  group_by(.rpc_method) |
  map({
    method: .[0].rpc_method,
    calls: length,
    p95: (sort_by(.duration_ms) | .[([length * 0.95 | floor, length - 1] | min)].duration_ms),
    impact: (length * (sort_by(.duration_ms) | .[([length * 0.95 | floor, length - 1] | min)].duration_ms))
  }) |
  sort_by(-.impact) |
  .[:10] |
  .[] |
  "\(.impact)\t\(.calls)\t\(.p95)\t\(.method)"
' "$LATENCY_FILE" | while IFS=$'\t' read -r impact calls p95 method; do
  printf "  %8s impact  (%s calls x %sms p95)  %s\n" "$impact" "$calls" "$p95" "$method"
done

echo ""

# --- Top offenders (total queries) --------------------------------------------
echo "=== Top 10 Query Offenders (by total queries) ==="
echo ""
jq -sr '
  group_by(.path) |
  map({path: .[0].path, calls: length, total: ([.[].queries] | add), avg: (([.[].queries] | add) / length | . * 10 | round / 10)}) |
  sort_by(-.total) |
  .[:10] |
  .[] |
  "\(.total)\t\(.calls)\t\(.avg)\t\(.path)"
' "$STATS_FILE" | while IFS=$'\t' read -r total calls avg path; do
  printf "  %6s total queries  (%s calls, avg %s/call)  %s\n" "$total" "$calls" "$avg" "$path"
done

echo ""

# --- pprof info ---------------------------------------------------------------
if [ -n "$PPROF_DIR" ]; then
  echo "=== pprof Profiles ==="
  echo ""
  if [ -f "$CPU_PROFILE" ] && [ -s "$CPU_PROFILE" ]; then
    echo "  CPU:  $CPU_PROFILE"
    echo "        go tool pprof -http=:8081 $CPU_PROFILE"
  else
    echo "  CPU:  (not captured — simulation may have been too short)"
  fi
  if [ -f "$HEAP_PROFILE" ] && [ -s "$HEAP_PROFILE" ]; then
    echo "  Heap: $HEAP_PROFILE"
    echo "        go tool pprof -http=:8081 $HEAP_PROFILE"
  else
    echo "  Heap: (not captured)"
  fi
  echo ""
fi

# --- Write markdown report ----------------------------------------------------
if [ -n "$OUTPUT_FILE" ]; then
  {
    echo "# Performance Profile"
    echo ""
    echo "- **Branch:** $BRANCH"
    echo "- **Commit:** $COMMIT_HASH"
    echo "- **Date:** $TIMESTAMP"
    echo "- **Scenario:** $SCENARIO (seed=$SEED, concurrency=$CONCURRENCY)"
    if [ -n "$READ_TRAFFIC" ]; then
      echo "- **Read traffic:** enabled (multiplier: ${READ_MULTIPLIER:-1.0})"
    fi
    echo "- **Total HTTP requests:** $TOTAL_REQUESTS"
    echo "- **Requests with DB:** $TOTAL_DB_REQUESTS"
    echo "- **Total SQL queries:** $TOTAL_QUERIES"
    echo "- **Total DB time:** ${TOTAL_DB_MS}ms"
    echo "- **Errors:** $ERROR_COUNT"
    echo "- **Wall-clock duration:** ${SIM_DURATION}s"
    echo ""

    echo "## Per-Endpoint Latency"
    echo ""
    echo "| RPC Method | Calls | P50ms | P95ms | P99ms | Max ms | Errors |"
    echo "|------------|------:|------:|------:|------:|-------:|-------:|"
    jq -sr '
      [.[] | select(.rpc_method != "")] |
      group_by(.rpc_method) |
      map({
        method: .[0].rpc_method,
        calls: length,
        errors: ([.[] | select(.status >= 400)] | length),
        p50: (sort_by(.duration_ms) | .[length * 0.5 | floor].duration_ms),
        p95: (sort_by(.duration_ms) | .[([length * 0.95 | floor, length - 1] | min)].duration_ms),
        p99: (sort_by(.duration_ms) | .[([length * 0.99 | floor, length - 1] | min)].duration_ms),
        max: ([.[].duration_ms] | max)
      }) |
      sort_by(-.calls) |
      .[] |
      "| \(.method) | \(.calls) | \(.p50) | \(.p95) | \(.p99) | \(.max) | \(.errors) |"
    ' "$LATENCY_FILE"
    echo ""

    echo "## Per-Endpoint DB Queries"
    echo ""
    echo "| Endpoint | Calls | Total Q | Min | Avg | P50 | P95 | Max |"
    echo "|----------|------:|--------:|----:|----:|----:|----:|----:|"
    jq -sr '
      group_by(.path) |
      map({
        path: .[0].path,
        calls: length,
        total: ([.[].queries] | add),
        min: ([.[].queries] | min),
        max: ([.[].queries] | max),
        avg: (([.[].queries] | add) / length | . * 10 | round / 10),
        p50: (sort_by(.queries) | .[length * 0.5 | floor].queries),
        p95: (sort_by(.queries) | .[([length * 0.95 | floor, length - 1] | min)].queries)
      }) |
      sort_by(-.total) |
      .[] |
      "| \(.path) | \(.calls) | \(.total) | \(.min) | \(.avg) | \(.p50) | \(.p95) | \(.max) |"
    ' "$STATS_FILE"
    echo ""

    echo "## Top 10 Latency Offenders (p95 x calls)"
    echo ""
    echo "| Rank | RPC Method | Impact | Calls | P95ms |"
    echo "|-----:|------------|-------:|------:|------:|"
    jq -sr '
      [.[] | select(.rpc_method != "")] |
      group_by(.rpc_method) |
      map({
        method: .[0].rpc_method,
        calls: length,
        p95: (sort_by(.duration_ms) | .[([length * 0.95 | floor, length - 1] | min)].duration_ms),
        impact: (length * (sort_by(.duration_ms) | .[([length * 0.95 | floor, length - 1] | min)].duration_ms))
      }) |
      sort_by(-.impact) |
      to_entries |
      .[:10] |
      .[] |
      "| \(.key + 1) | \(.value.method) | \(.value.impact) | \(.value.calls) | \(.value.p95) |"
    ' "$LATENCY_FILE"
    echo ""

    echo "## Top 10 Query Offenders"
    echo ""
    echo "| Rank | Endpoint | Total Q | Calls | Avg/Call |"
    echo "|-----:|----------|--------:|------:|--------:|"
    jq -sr '
      group_by(.path) |
      map({path: .[0].path, calls: length, total: ([.[].queries] | add), avg: (([.[].queries] | add) / length | . * 10 | round / 10)}) |
      sort_by(-.total) |
      to_entries |
      .[:10] |
      .[] |
      "| \(.key + 1) | \(.value.path) | \(.value.total) | \(.value.calls) | \(.value.avg) |"
    ' "$STATS_FILE"

    if [ -n "$PPROF_DIR" ]; then
      echo ""
      echo "## pprof Profiles"
      echo ""
      echo "- CPU: \`$CPU_PROFILE\`"
      echo "- Heap: \`$HEAP_PROFILE\`"
    fi
  } > "$OUTPUT_FILE"
  echo "Report written to $OUTPUT_FILE"
fi

# --- Comparison mode ----------------------------------------------------------
if [ -n "$COMPARE_FILE" ]; then
  if [ ! -f "$COMPARE_FILE" ]; then
    echo "Warning: comparison file not found: $COMPARE_FILE" >&2
  else
    echo "=== Comparison with $COMPARE_FILE ==="
    echo ""
    # Extract total queries from the previous report.
    PREV_QUERIES=$(grep -o 'Total SQL queries:\*\* [0-9]*' "$COMPARE_FILE" 2>/dev/null | grep -o '[0-9]*' || echo "")
    if [ -n "$PREV_QUERIES" ] && [ -n "$TOTAL_QUERIES" ]; then
      DELTA=$((TOTAL_QUERIES - PREV_QUERIES))
      if [ "$PREV_QUERIES" -gt 0 ]; then
        PCT=$(echo "scale=1; $DELTA * 100 / $PREV_QUERIES" | bc 2>/dev/null || echo "?")
        echo "  Total SQL queries: $PREV_QUERIES → $TOTAL_QUERIES (${DELTA:+$DELTA} / ${PCT}%)"
      fi
    fi

    PREV_REQUESTS=$(grep -o 'Total HTTP requests:\*\* [0-9]*' "$COMPARE_FILE" 2>/dev/null | grep -o '[0-9]*' || echo "")
    if [ -n "$PREV_REQUESTS" ] && [ -n "$TOTAL_REQUESTS" ]; then
      echo "  Total HTTP requests: $PREV_REQUESTS → $TOTAL_REQUESTS"
    fi

    PREV_ERRORS=$(grep -o 'Errors:\*\* [0-9]*' "$COMPARE_FILE" 2>/dev/null | grep -o '[0-9]*' || echo "")
    if [ -n "$PREV_ERRORS" ] && [ -n "$ERROR_COUNT" ]; then
      echo "  Errors: $PREV_ERRORS → $ERROR_COUNT"
    fi
    echo ""
  fi
fi

# --- Artifact locations -------------------------------------------------------
echo "Artifacts:"
echo "  Server log:    $SERVER_LOG"
echo "  Latency stats: $LATENCY_FILE"
echo "  DB stats:      $STATS_FILE"
if [ -n "$PPROF_DIR" ]; then
  echo "  CPU profile:   ${CPU_PROFILE:-n/a}"
  echo "  Heap profile:  ${HEAP_PROFILE:-n/a}"
fi
