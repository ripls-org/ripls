#!/usr/bin/env bash
# create_sa_key.sh — create one key for a service account in a project whose
# org policy forbids key creation (constraints/iam.disableServiceAccountKeyCreation).
#
#   scripts/create_sa_key.sh <project-id> <service-account-email> <out-file>
#
# Lifts the ban at project level, retries the key creation while that change
# propagates (usually one to two minutes), and re-enforces the ban on exit —
# including after a failure or Ctrl-C — so the window closes whatever happens.
# Re-enforcing restores a project-level policy that enforces the ban; run this
# only on a project where that was already the case.
#
# The key is written to <out-file> at mode 0600. Move it to where it is used and
# delete the local copy; see the key runbook in the deployment's secret docs.
set -uo pipefail

if [ $# -ne 3 ]; then
  echo "usage: $0 <project-id> <service-account-email> <out-file>" >&2
  exit 2
fi
PROJECT=$1
SA=$2
OUT=$3
CONSTRAINT=iam.disableServiceAccountKeyCreation

if [ -e "$OUT" ]; then
  echo "create_sa_key: $OUT already exists" >&2
  exit 1
fi
umask 077

reenforce() {
  if gcloud resource-manager org-policies enable-enforce "$CONSTRAINT" --project="$PROJECT" --quiet >/dev/null 2>&1; then
    echo "create_sa_key: $CONSTRAINT enforced again on $PROJECT" >&2
  else
    echo "create_sa_key: FAILED to re-enforce $CONSTRAINT on $PROJECT — run:" >&2
    echo "  gcloud resource-manager org-policies enable-enforce $CONSTRAINT --project=$PROJECT" >&2
  fi
}
trap reenforce EXIT

gcloud resource-manager org-policies disable-enforce "$CONSTRAINT" --project="$PROJECT" --quiet >/dev/null || exit 1
echo "create_sa_key: $CONSTRAINT lifted on $PROJECT; waiting for it to propagate" >&2

err=$(mktemp)
trap 'rm -f "$err"; reenforce' EXIT
for attempt in $(seq 1 36); do
  if gcloud iam service-accounts keys create "$OUT" --iam-account="$SA" --project="$PROJECT" --quiet 2>"$err"; then
    echo "create_sa_key: key written to $OUT (attempt $attempt)" >&2
    exit 0
  fi
  # Only the policy refusal is worth waiting out; anything else is a real error.
  if ! grep -q "$CONSTRAINT" "$err"; then
    cat "$err" >&2
    exit 1
  fi
  sleep 10
done
echo "create_sa_key: still refused after 6 minutes" >&2
exit 1
