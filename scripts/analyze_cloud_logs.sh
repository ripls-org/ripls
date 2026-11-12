#!/usr/bin/env bash
# analyze_cloud_logs.sh — Analyze Cloud Run request logs for traffic patterns.
#
# Usage:
#   # Analyze a pre-exported JSON log file:
#   ./scripts/analyze_cloud_logs.sh /tmp/cloud-run-dev-logs.json
#
#   # Export and analyze in one step:
#   ./scripts/analyze_cloud_logs.sh --export dev    # exports from the dev project
#   ./scripts/analyze_cloud_logs.sh --export prod   # exports from the prod project
#
#   # Compare dev and prod:
#   ./scripts/analyze_cloud_logs.sh --compare /tmp/cloud-run-dev-logs.json /tmp/cloud-run-prod-logs.json
#
#   # Save markdown report:
#   ./scripts/analyze_cloud_logs.sh /tmp/cloud-run-dev-logs.json --output docs/ai/production_traffic_analysis.md
#
#   # A self-hosted server's logs instead of Cloud Run's:
#   LOG_SOURCE=self_hosted ./scripts/analyze_cloud_logs.sh --export dev
set -euo pipefail

FRESHNESS="${FRESHNESS:-7d}"
LIMIT="${LIMIT:-50000}"

usage() {
    echo "Usage: $0 [--export dev|prod] [--compare FILE1 FILE2] [FILE] [--output FILE]"
    echo ""
    echo "Options:"
    echo "  --export ENV    Export logs from the dev or prod project, then analyze"
    echo "  --compare F1 F2 Compare two log files side by side"
    echo "  --output FILE   Write markdown report to file (default: stdout)"
    echo "  --freshness N   Log freshness for export (default: 7d)"
    echo "  --limit N       Max log entries to export (default: 50000)"
    exit 1
}

export_logs() {
    local env="$1"
    local project service outfile

    case "$env" in
        dev)
            project="$("$(dirname "$0")/gcp_project.sh" dev)"
            service="server-dev"
            ;;
        prod)
            project="$("$(dirname "$0")/gcp_project.sh" prod)"
            service="server"
            ;;
        *)
            echo "Error: unknown environment '$env' (use 'dev' or 'prod')" >&2
            exit 1
            ;;
    esac

    # Where the server's logs live, as in the monitoring module's log_source:
    # Cloud Run's, or a self-hosted server's, which its log shipper writes as
    # generic_task entries with job = the service name.
    case "${LOG_SOURCE:-cloud_run}" in
        cloud_run)   resource="resource.type=\"cloud_run_revision\" AND resource.labels.service_name=\"$service\"" ;;
        self_hosted) resource="resource.type=\"generic_task\" AND resource.labels.job=\"$service\"" ;;
        *)
            echo "Error: LOG_SOURCE must be 'cloud_run' or 'self_hosted'" >&2
            exit 1
            ;;
    esac

    outfile="/tmp/cloud-run-${env}-logs.json"
    echo "Exporting logs from $project/$service (source=${LOG_SOURCE:-cloud_run}, freshness=$FRESHNESS, limit=$LIMIT)..." >&2

    gcloud logging read \
        "$resource AND jsonPayload.message=\"http request\" AND NOT jsonPayload.path=\"/health\"" \
        --project="$project" \
        --format=json \
        --freshness="$FRESHNESS" \
        --limit="$LIMIT" \
        > "$outfile"

    echo "Exported to $outfile" >&2
    echo "$outfile"
}

analyze_logs() {
    local logfile="$1"
    local label="${2:-}"

    python3 - "$logfile" "$label" <<'PYEOF'
import json
import sys
import statistics
from collections import Counter, defaultdict
from datetime import datetime, timezone

logfile = sys.argv[1]
label = sys.argv[2] if len(sys.argv) > 2 and sys.argv[2] else logfile

with open(logfile) as f:
    data = json.load(f)

if not data:
    print(f"No log entries found in {logfile}")
    sys.exit(0)

# ── Classification ──────────────────────────────────────────────────────────
WRITE_PREFIXES = {
    'Save', 'Create', 'Update', 'Delete', 'Send', 'Express', 'Select',
    'Start', 'Complete', 'Cancel', 'Withdraw', 'Mark', 'RSVP', 'Offer',
    'Share', 'Add', 'Register', 'Fulfill', 'Remove', 'Reset', 'Set',
    'Accept', 'Reject', 'Ban', 'Unban', 'Report', 'Cleanup', 'Submit',
    'Stream', 'Invite', 'Leave', 'Join', 'Backfill', 'Migrate', 'Block',
    'Unblock', 'Archive', 'Unarchive', 'Mute', 'Unmute',
}

def extract_rpc(path):
    """Extract ServiceName/MethodName from /ripls.api.ServiceName/MethodName."""
    parts = path.strip('/').split('/')
    if len(parts) != 2:
        return None
    svc = parts[0].split('.')[-1]
    return f"{svc}/{parts[1]}"

def classify(method_name):
    """Classify an RPC method as READ or WRITE."""
    short = method_name.split('/')[-1]
    return 'WRITE' if any(short.startswith(p) for p in WRITE_PREFIXES) else 'READ'

def percentile(values, p):
    """Compute percentile from a sorted list."""
    if not values:
        return 0
    return statistics.quantiles(values, n=100)[min(p - 1, len(statistics.quantiles(values, n=100)) - 1)]

# ── Parse entries ───────────────────────────────────────────────────────────
rpc_durations = defaultdict(list)   # method → [duration_ms, ...]
rpc_statuses = defaultdict(Counter) # method → {status: count}
user_sessions = defaultdict(list)   # user_id → [(timestamp, method), ...]
timestamps = []
errors_by_method = Counter()

for entry in data:
    jp = entry.get('jsonPayload', {})
    path = jp.get('path', '')
    rpc = extract_rpc(path)
    if not rpc:
        continue

    duration = jp.get('duration_ms', 0)
    status = jp.get('status', 0)
    user_id = jp.get('user_id', '')
    ts_str = entry.get('timestamp', '')

    rpc_durations[rpc].append(duration)
    rpc_statuses[rpc][status] += 1

    if status >= 400:
        errors_by_method[rpc] += 1

    if user_id:
        user_sessions[user_id].append((ts_str, rpc))

    if ts_str:
        try:
            ts = datetime.fromisoformat(ts_str.replace('Z', '+00:00'))
            timestamps.append(ts)
        except (ValueError, TypeError):
            pass

# ── Compute metrics ─────────────────────────────────────────────────────────
total_calls = sum(len(v) for v in rpc_durations.values())
read_calls = sum(len(v) for m, v in rpc_durations.items() if classify(m) == 'READ')
write_calls = total_calls - read_calls

# Per-method stats
method_stats = []
for method, durations in rpc_durations.items():
    durations_sorted = sorted(durations)
    count = len(durations)
    p50 = percentile(durations_sorted, 50)
    p95 = percentile(durations_sorted, 95)
    p99 = percentile(durations_sorted, 99)
    error_count = errors_by_method.get(method, 0)
    error_rate = 100 * error_count / count if count > 0 else 0
    rw = classify(method)
    method_stats.append({
        'method': method,
        'count': count,
        'pct': 100 * count / total_calls,
        'rw': rw,
        'p50': p50,
        'p95': p95,
        'p99': p99,
        'errors': error_count,
        'error_rate': error_rate,
    })

method_stats.sort(key=lambda x: x['count'], reverse=True)

# Peak RPS (requests per second, bucketed by minute)
rps_by_minute = Counter()
for ts in timestamps:
    minute_key = ts.strftime('%Y-%m-%d %H:%M')
    rps_by_minute[minute_key] += 1

peak_minute = max(rps_by_minute.values()) if rps_by_minute else 0
peak_rps = peak_minute / 60.0

# Time range
if timestamps:
    ts_sorted = sorted(timestamps)
    time_range_start = ts_sorted[0].strftime('%Y-%m-%d %H:%M')
    time_range_end = ts_sorted[-1].strftime('%Y-%m-%d %H:%M')
    time_range_hours = (ts_sorted[-1] - ts_sorted[0]).total_seconds() / 3600
else:
    time_range_start = time_range_end = "unknown"
    time_range_hours = 0

# ── User session patterns ──────────────────────────────────────────────────
session_sequences = []
for user_id, events in user_sessions.items():
    events.sort(key=lambda x: x[0])
    methods = [e[1] for e in events]
    session_sequences.append(methods)

# Common opening sequences (first 5 RPCs of a session)
opening_seqs = Counter()
for seq in session_sequences:
    opening = tuple(seq[:5])
    opening_seqs[opening] += 1

# ── Output ──────────────────────────────────────────────────────────────────
print(f"# Traffic Analysis: {label}")
print()
print(f"**Period:** {time_range_start} to {time_range_end} ({time_range_hours:.1f} hours)")
print(f"**Total RPC calls:** {total_calls:,}")
print(f"**Unique RPC methods:** {len(rpc_durations)}")
print(f"**Read calls:** {read_calls:,} ({100*read_calls/total_calls:.1f}%)")
print(f"**Write calls:** {write_calls:,} ({100*write_calls/total_calls:.1f}%)")
print(f"**Read/Write ratio:** {read_calls/max(write_calls,1):.2f}:1")
print(f"**Peak RPS:** {peak_rps:.1f} (busiest minute: {peak_minute} requests)")
if user_sessions:
    print(f"**Unique users:** {len(user_sessions)}")
print()

print("## RPC Method Frequency")
print()
print(f"| {'RPC Method':<48s} | {'Calls':>6s} | {'%':>5s} | {'R/W':>5s} | {'p50ms':>5s} | {'p95ms':>5s} | {'p99ms':>5s} | {'Err%':>4s} |")
print(f"|{'-'*50}|{'-'*8}|{'-'*7}|{'-'*7}|{'-'*7}|{'-'*7}|{'-'*7}|{'-'*6}|")
for s in method_stats:
    print(f"| {s['method']:<48s} | {s['count']:>6d} | {s['pct']:>4.1f}% | {s['rw']:>5s} | {s['p50']:>5.0f} | {s['p95']:>5.0f} | {s['p99']:>5.0f} | {s['error_rate']:>3.1f}% |")

print()

# Read-only summary
read_stats = [s for s in method_stats if s['rw'] == 'READ']
write_stats = [s for s in method_stats if s['rw'] == 'WRITE']

print("## Read Endpoints (sorted by frequency)")
print()
print(f"| {'RPC Method':<48s} | {'Calls':>6s} | {'% of Reads':>10s} | {'p50ms':>5s} | {'p95ms':>5s} |")
print(f"|{'-'*50}|{'-'*8}|{'-'*12}|{'-'*7}|{'-'*7}|")
for s in read_stats:
    pct_reads = 100 * s['count'] / max(read_calls, 1)
    print(f"| {s['method']:<48s} | {s['count']:>6d} | {pct_reads:>9.1f}% | {s['p50']:>5.0f} | {s['p95']:>5.0f} |")

print()
print("## Write Endpoints (sorted by frequency)")
print()
print(f"| {'RPC Method':<48s} | {'Calls':>6s} | {'% of Writes':>11s} | {'p50ms':>5s} | {'p95ms':>5s} |")
print(f"|{'-'*50}|{'-'*8}|{'-'*13}|{'-'*7}|{'-'*7}|")
for s in write_stats:
    pct_writes = 100 * s['count'] / max(write_calls, 1)
    print(f"| {s['method']:<48s} | {s['count']:>6d} | {pct_writes:>10.1f}% | {s['p50']:>5.0f} | {s['p95']:>5.0f} |")

print()

# Error summary
error_stats = [s for s in method_stats if s['errors'] > 0]
if error_stats:
    error_stats.sort(key=lambda x: x['errors'], reverse=True)
    print("## Error Rates")
    print()
    print(f"| {'RPC Method':<48s} | {'Errors':>6s} | {'Total':>6s} | {'Rate':>5s} |")
    print(f"|{'-'*50}|{'-'*8}|{'-'*8}|{'-'*7}|")
    for s in error_stats:
        print(f"| {s['method']:<48s} | {s['errors']:>6d} | {s['count']:>6d} | {s['error_rate']:>4.1f}% |")
    print()

# Latency outliers (p95 > 500ms)
slow_methods = [s for s in method_stats if s['p95'] > 500]
if slow_methods:
    slow_methods.sort(key=lambda x: x['p95'], reverse=True)
    print("## Slow Endpoints (p95 > 500ms)")
    print()
    print(f"| {'RPC Method':<48s} | {'p50ms':>5s} | {'p95ms':>5s} | {'p99ms':>5s} | {'Calls':>6s} |")
    print(f"|{'-'*50}|{'-'*7}|{'-'*7}|{'-'*7}|{'-'*8}|")
    for s in slow_methods:
        print(f"| {s['method']:<48s} | {s['p50']:>5.0f} | {s['p95']:>5.0f} | {s['p99']:>5.0f} | {s['count']:>6d} |")
    print()

# User session patterns
if session_sequences:
    print("## User Session Patterns")
    print()
    print(f"Users with tracked sessions: {len(session_sequences)}")
    avg_session_len = statistics.mean(len(s) for s in session_sequences)
    print(f"Average session length: {avg_session_len:.1f} RPCs")
    print()
    if opening_seqs:
        print("Most common opening sequences (first 5 RPCs):")
        print()
        for seq, count in opening_seqs.most_common(10):
            print(f"  {count}x: {' → '.join(seq)}")
        print()

PYEOF
}

# ── Main ────────────────────────────────────────────────────────────────────
OUTPUT=""
MODE="analyze"
FILES=()

while [[ $# -gt 0 ]]; do
    case "$1" in
        --export)
            MODE="export"
            ENV="$2"
            shift 2
            ;;
        --compare)
            MODE="compare"
            FILES+=("$2" "$3")
            shift 3
            ;;
        --output)
            OUTPUT="$2"
            shift 2
            ;;
        --freshness)
            FRESHNESS="$2"
            shift 2
            ;;
        --limit)
            LIMIT="$2"
            shift 2
            ;;
        -h|--help)
            usage
            ;;
        *)
            FILES+=("$1")
            shift
            ;;
    esac
done

run_analysis() {
    case "$MODE" in
        export)
            local logfile
            logfile=$(export_logs "$ENV")
            analyze_logs "$logfile" "$ENV"
            ;;
        compare)
            if [[ ${#FILES[@]} -ne 2 ]]; then
                echo "Error: --compare requires exactly 2 files" >&2
                exit 1
            fi
            echo "═══════════════════════════════════════════════════════════════"
            analyze_logs "${FILES[0]}" "${FILES[0]}"
            echo ""
            echo "═══════════════════════════════════════════════════════════════"
            echo ""
            analyze_logs "${FILES[1]}" "${FILES[1]}"
            ;;
        analyze)
            if [[ ${#FILES[@]} -ne 1 ]]; then
                echo "Error: provide exactly 1 log file to analyze" >&2
                usage
            fi
            analyze_logs "${FILES[0]}"
            ;;
    esac
}

if [[ -n "$OUTPUT" ]]; then
    run_analysis > "$OUTPUT"
    echo "Report written to $OUTPUT" >&2
else
    run_analysis
fi
