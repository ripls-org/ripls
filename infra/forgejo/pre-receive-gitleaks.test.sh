#!/usr/bin/env bash
# Tests for pre-receive-gitleaks: real pushes into a throwaway bare repository
# with the hook installed.
#
#   GITLEAKS=/path/to/gitleaks ./pre-receive-gitleaks.test.sh
#
# Needs git and a gitleaks binary (GITLEAKS, or `gitleaks` on PATH). The fake
# secret is made up at run time, so this file holds nothing a scanner would
# flag.
set -euo pipefail

HERE="$(cd "$(dirname "$0")" && pwd)"
GITLEAKS="${GITLEAKS:-$(command -v gitleaks || true)}"
[ -n "$GITLEAKS" ] || { echo "no gitleaks binary: set GITLEAKS or put gitleaks on PATH" >&2; exit 2; }

tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT
export GIT_CONFIG_GLOBAL=/dev/null GIT_CONFIG_SYSTEM=/dev/null
export GIT_AUTHOR_NAME=t GIT_AUTHOR_EMAIL=t@example.invalid
export GIT_COMMITTER_NAME=t GIT_COMMITTER_EMAIL=t@example.invalid

# The shape gitleaks' built-in rules know as a GitHub personal access token.
# `head` first: with `tr … | head -c` the early exit of head would kill tr with
# SIGPIPE and, under pipefail, this script.
fake_secret="ghp_$(head -c 2048 /dev/urandom | LC_ALL=C tr -dc 'A-Za-z0-9' | cut -c1-36)"

pass=0
fail=0
check() { # check <description> <expected: ok|refused> <actual exit code>
  local want="$2" got="refused"
  [ "$3" -eq 0 ] && got="ok"
  if [ "$want" = "$got" ]; then
    pass=$((pass + 1)); echo "ok    $1"
  else
    fail=$((fail + 1)); echo "FAIL  $1 (wanted $want, push was $got)"
  fi
}

# new_remote <name> [gitleaks path]: a bare repository with the hook wired the
# way Forgejo wires it, as a file in hooks/pre-receive.d run by hooks/pre-receive.
new_remote() {
  local dir="$tmp/$1.git" bin="${2:-$GITLEAKS}"
  git init -q --bare "$dir"
  mkdir -p "$dir/hooks/pre-receive.d"
  cp "$HERE/pre-receive-gitleaks" "$dir/hooks/pre-receive.d/gitleaks"
  chmod +x "$dir/hooks/pre-receive.d/gitleaks"
  cat >"$dir/hooks/pre-receive" <<EOF
#!/usr/bin/env bash
export GITLEAKS="$bin"
data=\$(cat)
for hook in ./hooks/pre-receive.d/*; do
  echo "\$data" | "\$hook" || exit 1
done
EOF
  chmod +x "$dir/hooks/pre-receive"
  echo "$dir"
}

new_clone() { # new_clone <name> <remote>
  local dir="$tmp/$1"
  git init -q -b main "$dir"
  git -C "$dir" remote add origin "$2"
  echo "$dir"
}

commit() { # commit <clone> <file> <content> <message>
  printf '%s\n' "$3" >"$1/$2"
  git -C "$1" add -A
  git -C "$1" commit -q -m "$4"
}

push() { git -C "$1" push -q origin "${@:2}" >"$tmp/push.log" 2>&1; }

# 1. A clean push is accepted.
remote="$(new_remote one)"
work="$(new_clone one-work "$remote")"
commit "$work" README.md "hello" "first"
rc=0; push "$work" main || rc=$?
check "a clean first push is accepted" ok "$rc"

# 2. A push that adds a secret is refused, and the output does not repeat it.
commit "$work" config.env "GITHUB_TOKEN=$fake_secret" "add config"
rc=0; push "$work" main || rc=$?
check "a push that adds a secret is refused" refused "$rc"
if grep -q "$fake_secret" "$tmp/push.log"; then
  fail=$((fail + 1)); echo "FAIL  the refusal printed the secret"
else
  pass=$((pass + 1)); echo "ok    the refusal does not print the secret"
fi
if [ -n "$(git -C "$remote" rev-list --all -- config.env)" ]; then
  fail=$((fail + 1)); echo "FAIL  the refused commit reached the repository"
else
  pass=$((pass + 1)); echo "ok    the refused commit is not in the repository"
fi

# 3. Deleting the secret in a later commit is not enough: it is still in history.
git -C "$work" rm -q config.env
git -C "$work" commit -q -m "remove config"
rc=0; push "$work" main || rc=$?
check "a secret deleted by a later commit is still refused" refused "$rc"

# 4. Rewriting the commits so the secret is gone is accepted.
git -C "$work" reset -q --hard HEAD~2
commit "$work" notes.md "nothing to see" "notes"
rc=0; push "$work" main || rc=$?
check "the same branch without the secret is accepted" ok "$rc"

# 5. A secret on a new branch is refused too.
git -C "$work" checkout -q -b feature
commit "$work" config.env "GITHUB_TOKEN=$fake_secret" "feature config"
rc=0; push "$work" feature || rc=$?
check "a secret on a new branch is refused" refused "$rc"

# 6. A fingerprint in .gitleaksignore, in the same push, lets it through.
"$GITLEAKS" git --no-banner --redact --report-format json --report-path "$tmp/report.json" \
  --exit-code 0 "$work" >/dev/null 2>&1
fingerprint="$(sed -n 's/.*"Fingerprint": *"\([^"]*\)".*/\1/p' "$tmp/report.json" | head -1)"
commit "$work" .gitleaksignore "$fingerprint" "allow the fixture"
rc=0; push "$work" feature || rc=$?
check "a secret listed in .gitleaksignore in the same push is accepted" ok "$rc"

# 7. Deleting a branch, and pointing a new branch at commits the repository
#    already has, scan nothing and are accepted.
rc=0; push "$work" --delete feature || rc=$?
check "deleting a branch is accepted" ok "$rc"
rc=0; push "$work" main:refs/heads/copy || rc=$?
check "a new branch at existing commits is accepted" ok "$rc"

# 8. With no gitleaks binary the hook refuses the push and says why.
remote2="$(new_remote two "$tmp/no-such-gitleaks")"
work2="$(new_clone two-work "$remote2")"
commit "$work2" README.md "hello" "first"
rc=0; push "$work2" main || rc=$?
check "a missing gitleaks binary refuses the push" refused "$rc"
if grep -q "not installed" "$tmp/push.log"; then
  pass=$((pass + 1)); echo "ok    and the message says gitleaks is missing"
else
  fail=$((fail + 1)); echo "FAIL  the message does not say gitleaks is missing"
fi

echo
echo "$pass passed, $fail failed"
[ "$fail" -eq 0 ]
