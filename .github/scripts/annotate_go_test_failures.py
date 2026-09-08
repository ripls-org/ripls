#!/usr/bin/env python3
"""Read a `go test -json` stream and emit GitHub Actions ::error:: workflow
commands for every failing test, one per `file.go:LINE:` location found in
the test's output. The commands attach as inline annotations on the PR diff
at the corresponding repo-relative file/line.

Usage: annotate_go_test_failures.py <test-output.json>
"""

import json
import re
import sys

MODULE_PREFIX = "go.ripls.org/ripls/"
LOC_PATTERN = re.compile(r"^\s+([^\s:]+\.go):(\d+):\s*(.*)$")


def gh_escape(s: str) -> str:
    """Escape a string for inclusion in a GitHub Actions workflow command."""
    return s.replace("%", "%25").replace("\r", "%0D").replace("\n", "%0A")


def main(path: str) -> int:
    outputs: dict[tuple[str, str], list[str]] = {}
    failed: set[tuple[str, str]] = set()

    with open(path, encoding="utf-8") as f:
        for line in f:
            line = line.strip()
            if not line:
                continue
            try:
                event = json.loads(line)
            except json.JSONDecodeError:
                continue
            test = event.get("Test")
            pkg = event.get("Package")
            action = event.get("Action")
            if not test or not pkg:
                continue
            key = (pkg, test)
            if action == "output":
                outputs.setdefault(key, []).append(event.get("Output", ""))
            elif action == "fail":
                failed.add(key)

    if failed:
        # Hint that the upstream artifact has the unfiltered context. Some
        # readers (humans skimming, the Claude PR-feedback bot) may not realise
        # gotestfmt's --hide=successful-tests dropped passing-test breadcrumbs
        # from this step's log, but those breadcrumbs are present in full in
        # the test-output.json artifact uploaded by the same job.
        print(
            "::notice title=Full test output available as artifact::"
            "This log step hides passing tests via gotestfmt's "
            "--hide=successful-tests. The complete unfiltered go test -json "
            "stream (every passing test, every t.Log line, every timing) is "
            "uploaded as a workflow run artifact named go-test-output* on "
            "this run page. Download with: gh run download <run-id> -n "
            "<artifact-name>"
        )

    annotations = 0
    for pkg, test in sorted(failed):
        if not pkg.startswith(MODULE_PREFIX):
            continue
        rel_dir = pkg[len(MODULE_PREFIX):]
        # The Go test JSON stream does not distinguish t.Log from t.Errorf /
        # t.Fatalf — both produce identical "    file.go:LINE: message" events.
        # We keep only the LAST file:LINE: line in a failing test's output to
        # annotate the line that actually triggered the failure (the last
        # t.Errorf / t.Fatalf almost always comes last) and skip earlier t.Log
        # breadcrumbs that would otherwise be annotated as errors.
        last_match = None
        for chunk in outputs.get((pkg, test), []):
            for raw in chunk.splitlines():
                match = LOC_PATTERN.match(raw)
                if match:
                    last_match = match
        if last_match is None:
            continue
        file_, lineno, message = last_match.group(1), last_match.group(2), last_match.group(3)
        title = gh_escape(test).replace(",", "%2C")
        print(
            f"::error file={rel_dir}/{file_},"
            f"line={lineno},"
            f"title={title}::"
            f"{gh_escape(message)}"
        )
        annotations += 1

    print(
        f"Emitted {annotations} annotation(s) for {len(failed)} failing test(s)",
        file=sys.stderr,
    )
    return 0


if __name__ == "__main__":
    if len(sys.argv) != 2:
        print(f"Usage: {sys.argv[0]} <test-output.json>", file=sys.stderr)
        sys.exit(2)
    sys.exit(main(sys.argv[1]))
