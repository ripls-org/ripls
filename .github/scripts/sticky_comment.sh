#!/usr/bin/env bash
# sticky_comment.sh — keep ONE comment per topic on a pull request.
#
#   sticky_comment.sh <pr number> <topic> <<< "comment body"
#
# The first call for a topic posts a comment; later calls rewrite that comment
# instead of adding another, so a pull request that is pushed to twenty times
# carries one "staging website deployed" note, not twenty. The comment is found
# again by an HTML marker on its first line, which the forge does not render.
#
# It talks to the forge's REST API directly. The two calls it needs (list and
# edit an issue comment) have the same paths and shapes on Forgejo and GitHub,
# and Actions provides the base URL and repository on both. It replaced a
# marketplace action that only spoke to GitHub (#3293).
#
# Reads: GITHUB_API_URL, GITHUB_REPOSITORY, and a token in GH_TOKEN or
# GITHUB_TOKEN that may comment on pull requests.
set -euo pipefail

pr="${1:?usage: sticky_comment.sh <pr number> <topic>}"
topic="${2:?usage: sticky_comment.sh <pr number> <topic>}"
token="${GH_TOKEN:-${GITHUB_TOKEN:?set GH_TOKEN or GITHUB_TOKEN}}"
api="${GITHUB_API_URL:?}/repos/${GITHUB_REPOSITORY:?}"

if [[ ! "$pr" =~ ^[0-9]+$ ]] || [[ ! "$topic" =~ ^[a-z0-9-]+$ ]]; then
  echo "sticky_comment.sh: the pull request must be a number and the topic [a-z0-9-]" >&2
  exit 2
fi

marker="<!-- sticky:${topic} -->"
payload="$(jq -Rs --arg marker "$marker" '{body: ($marker + "\n" + .)}')"

call() {
  curl -fsS -H "Authorization: token $token" -H 'Content-Type: application/json' "$@"
}

# The first hundred comments are enough: the sticky comment is posted by the
# first run on the pull request, so it is always near the top.
existing="$(call "$api/issues/$pr/comments?per_page=100&limit=100" |
  jq -r --arg marker "$marker" '[.[] | select(.body | startswith($marker))][0].id // empty')"

if [ -n "$existing" ]; then
  call -X PATCH "$api/issues/comments/$existing" -d "$payload" >/dev/null
  echo "updated comment $existing on #$pr"
else
  call -X POST "$api/issues/$pr/comments" -d "$payload" >/dev/null
  echo "posted a comment on #$pr"
fi
