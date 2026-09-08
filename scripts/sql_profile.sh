#!/usr/bin/env bash
# sql_profile.sh — Measure per-endpoint SQL query counts using the simulation.
#
# Starts the server with JSON logging, runs the college-friends simulation
# scenario, then extracts per-endpoint query statistics from the server log.
# Run before and after optimizations to compare query counts.
#
# Prerequisites:
#   - Local PostgreSQL running (npm run db:start)
#   - Generated code exists (npm run generate)
#   - jq installed
#
# Usage:
#   ./scripts/sql_profile.sh                    # run and print summary
#   ./scripts/sql_profile.sh -o baseline.md     # also write markdown report
#
# The script saves raw logs to /tmp/sql-baseline-server.log for inspection.

set -euo pipefail
cd "$(git rev-parse --show-toplevel)"

# --- Parse flags -----------------------------------------------------------
OUTPUT_FILE=""
while getopts "o:" opt; do
  case $opt in
    o) OUTPUT_FILE="$OPTARG" ;;
    *) echo "Usage: $0 [-o output.md]" >&2; exit 1 ;;
  esac
done

# --- Check prerequisites ---------------------------------------------------
if ! command -v jq &>/dev/null; then
  echo "Error: jq is required. Install with: brew install jq" >&2
  exit 1
fi

SERVER_LOG="/tmp/sql-baseline-server.log"
COMMIT_HASH=$(git rev-parse --short HEAD)
BRANCH=$(git branch --show-current)
TIMESTAMP=$(date -u +"%Y-%m-%dT%H:%M:%SZ")
SCENARIO="college-friends"
SEED=42

echo "=== SQL Query Baseline ==="
echo "Branch: $BRANCH  Commit: $COMMIT_HASH"
echo "Scenario: $SCENARIO  Seed: $SEED"
echo ""

# --- Build server -----------------------------------------------------------
echo "Building server..."
go build -o tmp/server ./server

# --- Start server -----------------------------------------------------------
echo "Starting server (log: $SERVER_LOG)..."
# JWT signing secret comes from Secret Manager (dev project) via gcloud ADC.
JWT_SECRET=$(./scripts/fetch_secret.sh dev jwt-signing-secret)

./tmp/server \
  --log-level=debug \
  --log-format=json \
  --dev-mode \
  --db=postgres://ripls:ripls_dev@localhost:5432/ripls?sslmode=disable \
  --local-media-storage=./media_storage \
  --notification-provider=noop \
  --invite-link-hostname=localhost:8080 \
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
  if curl -sf http://localhost:8080/health > /dev/null 2>&1; then
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

# --- Purge previous simulation data ----------------------------------------
echo "Purging previous simulation data..."
go run ./server/cmd/simulate -url http://localhost:8080 -purge 2>&1 || true

# --- Run simulation ---------------------------------------------------------
echo "Running simulation ($SCENARIO, seed=$SEED)..."
go run ./server/cmd/simulate \
  -url http://localhost:8080 \
  -assets-dir server/simulation/assets \
  -scenario "$SCENARIO" \
  -seed "$SEED"

echo ""
echo "Simulation complete. Extracting stats..."

# --- Stop server to flush logs ----------------------------------------------
kill "$SERVER_PID" 2>/dev/null || true
wait "$SERVER_PID" 2>/dev/null || true
sleep 1

# --- Extract stats ----------------------------------------------------------
# Filter for lines with db_queries, extract path and count.
# The JSON log has: {"message":"request db stats","db_queries":N,"db_duration_ms":M,"path":"/..."}

STATS_FILE="/tmp/sql-baseline-stats.jsonl"
grep '"db_queries"' "$SERVER_LOG" | jq -c '{path: .path, queries: .db_queries, duration_ms: .db_duration_ms}' > "$STATS_FILE" 2>/dev/null || true

TOTAL_REQUESTS=$(wc -l < "$STATS_FILE" | tr -d ' ')
if [ "$TOTAL_REQUESTS" -eq 0 ]; then
  echo "Warning: No db_queries entries found in log. Is the QueryStats middleware wired in?"
  echo "Check $SERVER_LOG for details."
  exit 1
fi

TOTAL_QUERIES=$(jq -s '[.[].queries] | add' "$STATS_FILE")

echo "=== Summary ==="
echo "Total requests with DB activity: $TOTAL_REQUESTS"
echo "Total SQL queries: $TOTAL_QUERIES"
echo ""

# Per-endpoint summary: count, total queries, min, max, avg, p50, p95.
echo "=== Per-Endpoint Stats ==="
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

# --- Top offenders (sorted by total queries) --------------------------------
echo "=== Top 10 Offenders (by total queries) ==="
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

# --- Write markdown report if -o specified ----------------------------------
if [ -n "$OUTPUT_FILE" ]; then
  {
    echo "# SQL Query Baseline"
    echo ""
    echo "- **Branch:** $BRANCH"
    echo "- **Commit:** $COMMIT_HASH"
    echo "- **Date:** $TIMESTAMP"
    echo "- **Scenario:** $SCENARIO (seed=$SEED)"
    echo "- **Total requests with DB queries:** $TOTAL_REQUESTS"
    echo "- **Total SQL queries:** $TOTAL_QUERIES"
    echo ""
    echo "## Per-Endpoint Stats"
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
    echo "## Top 10 Offenders"
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
  } > "$OUTPUT_FILE"
  echo "Report written to $OUTPUT_FILE"
fi

echo "Raw server log: $SERVER_LOG"
echo "Stats JSONL: $STATS_FILE"
