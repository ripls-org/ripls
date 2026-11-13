#!/usr/bin/env bash
# ci_failure_hook.sh — hand a failed CI run to its fixer, on a forge that has no
# `workflow_run` trigger (#3293).
#
# On GitHub, claude_fix_ci_failure and claude_fix_renovate_ci listen for
# `workflow_run`, and this script does nothing. Forgejo has no such trigger, so
# each watched workflow ends with a `fixer` job that runs this when an earlier
# job failed on a pull request.
#
# It starts the matching dispatcher by workflow_dispatch: claude/* heads go to
# claude_fix_ci_failure, renovate/* heads to claude_fix_renovate_ci. Everything
# those dispatchers check (open PR, same-repo head, author, holds, attempt cap)
# they still check; this only says "this run, on this PR, failed".
#
# Why a script the job fetches, and not a reusable workflow: inside a called
# workflow Forgejo reports the event as `workflow_call`, so the pull request
# this is about is no longer in reach.
#
# Environment, all from the calling job:
#   TOKEN            the job's own token; on Forgejo it may dispatch a workflow
#                    in its own repository
#   HEAD_REF         github.head_ref
#   HEAD_REPO        github.event.pull_request.head.repo.full_name
#   PR_NUMBER        github.event.pull_request.number
#   DEFAULT_BRANCH   the branch whose copy of the dispatcher runs
set -euo pipefail

if [ "${GITHUB_SERVER_URL:-}" = "https://github.com" ]; then
  echo "::notice::GitHub starts the fixers from workflow_run; nothing to do here."
  exit 0
fi
if [ -z "${PR_NUMBER:-}" ] || [ "${HEAD_REPO:-}" != "$GITHUB_REPOSITORY" ]; then
  echo "::notice::Not a same-repository pull request; no fixer."
  exit 0
fi
case "${HEAD_REF:-}" in
  claude/*) fixer=claude_fix_ci_failure.yaml ;;
  renovate/*) fixer=claude_fix_renovate_ci.yaml ;;
  *)
    echo "::notice::${HEAD_REF:-?} is neither claude/* nor renovate/*; no fixer."
    exit 0
    ;;
esac

jq -nc --arg ref "${DEFAULT_BRANCH:-main}" --arg pr "$PR_NUMBER" --arg run "$GITHUB_RUN_ID" \
  '{ref: $ref, inputs: {pr_number: $pr, failed_run_id: $run}}' |
  curl -fsS -X POST -H "Authorization: token $TOKEN" -H "Content-Type: application/json" \
    --data-binary @- \
    "$GITHUB_API_URL/repos/$GITHUB_REPOSITORY/actions/workflows/$fixer/dispatches"
echo "::notice::Asked $fixer to look at run $GITHUB_RUN_ID on #$PR_NUMBER."
