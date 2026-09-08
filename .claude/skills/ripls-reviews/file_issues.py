#!/usr/bin/env python3
"""Parse `## Issues to File` sections from review reports and create GitHub issues.

Used by the `/ripls-reviews` skill. Each filed issue carries:
- `auto-filed` label (identifies all auto-generated review bugs)
- `review:<name>` label (per-review provenance)
- a `<!-- review-fingerprint: ... -->` HTML comment in the body for dedup

Usage:
    file_issues.py [--apply] [--severities=Critical,High,Medium]

Without `--apply`, runs in dry-run mode and prints what would be filed.
"""
from __future__ import annotations

import argparse
import json
import os
import re
import subprocess
import sys
import tempfile
from pathlib import Path

REPO_ROOT = Path(__file__).resolve().parents[3]
REPORTS_DIR = REPO_ROOT / "docs" / "reviews" / "reports"


def parse_review_name(report_path: Path) -> str:
    return report_path.stem.removesuffix("_review")


def extract_issues_section(text: str) -> str:
    m = re.search(r"^## Issues to File\s*\n(.*)\Z", text, re.MULTILINE | re.DOTALL)
    if not m:
        return ""
    body = m.group(1)
    if "_None this run._" in body[:200]:
        return ""
    return body


def parse_findings(section: str) -> list[dict]:
    findings = []
    chunks = re.split(r"^### ", section, flags=re.MULTILINE)
    for chunk in chunks[1:]:
        lines = chunk.split("\n", 1)
        heading = lines[0].strip()
        body = lines[1] if len(lines) > 1 else ""
        sev_m = re.match(r"\[(\w+)\]\s*(.*)", heading)
        if not sev_m:
            continue
        severity = sev_m.group(1)
        title = sev_m.group(2).strip()

        def field(name: str) -> str | None:
            pattern = (
                rf"\*\*{re.escape(name)}\*\*\s*:\s*"
                rf"(.*?)(?=\n\s*-\s*\*\*|\n\n###|\n\n##|\Z)"
            )
            m = re.search(pattern, body, re.DOTALL)
            return m.group(1).strip() if m else None

        fingerprint = field("Fingerprint")
        if fingerprint:
            fingerprint = fingerprint.strip("`").strip()

        findings.append({
            "severity": severity,
            "title": title,
            "fingerprint": fingerprint,
            "type": (field("Type") or "").strip(),
            "why": field("Why it matters") or "",
            "locations": field("Locations") or "",
            "recommendation": field("Recommendation") or "",
            "suggested_labels": field("Suggested labels"),
        })
    return findings


def fetch_existing_labels() -> set[str]:
    out = subprocess.run(
        ["gh", "label", "list", "--limit", "500", "--json", "name"],
        capture_output=True, text=True, check=True,
    )
    return {item["name"] for item in json.loads(out.stdout)}


def _normalize_title(title: str) -> str:
    """Lowercase, drop a leading `[review_name]` tag, collapse whitespace.

    Used as a fallback dedup key when fingerprints drift between runs.
    """
    t = re.sub(r"^\[[^\]]+\]\s*", "", title).lower()
    return re.sub(r"\s+", " ", t).strip()


def fetch_existing_issues() -> tuple[dict[str, int], dict[str, int]]:
    """Return (fingerprint→issue#, normalized_title→issue#) for open auto-filed issues.

    Closed issues are excluded so a regression of a previously-fixed finding files cleanly.
    """
    out = subprocess.run(
        ["gh", "issue", "list", "--label", "auto-filed", "--state", "open",
         "--limit", "1000", "--json", "number,title,body"],
        capture_output=True, text=True, check=True,
    )
    issues = json.loads(out.stdout)
    fp_map: dict[str, int] = {}
    title_map: dict[str, int] = {}
    for issue in issues:
        body = issue.get("body") or ""
        m = re.search(r"review-fingerprint:\s*(\S+)", body)
        if m:
            fp_map[m.group(1).strip().rstrip(">").strip()] = issue["number"]
        title = issue.get("title") or ""
        if title:
            title_map[_normalize_title(title)] = issue["number"]
    return fp_map, title_map


def build_body(review_name: str, finding: dict) -> str:
    report_path = f"docs/reviews/reports/{review_name}_review.md"
    locs = finding["locations"]
    loc_lines = [l.strip().strip("`") for l in re.split(r",\s*", locs) if l.strip()]
    loc_md = "\n".join(f"- `{l}`" for l in loc_lines) if loc_lines else "_(unspecified)_"
    suggested = finding.get("suggested_labels") or ""
    suggested_md = f"\n**Suggested labels:** {suggested}" if suggested else ""
    return f"""**Severity:** {finding['severity']}
**Source review:** [`{report_path}`](../{report_path})
**Type hint:** {finding['type'] or '(unset)'}{suggested_md}

## Why it matters

{finding['why']}

## Locations

{loc_md}

## Recommendation

{finding['recommendation']}

---

_Auto-filed by `/ripls-reviews`. Priority will be assigned by `/ripls-triage`._

<!-- review-fingerprint: {finding['fingerprint']} -->
"""


def create_issue(
    review_name: str,
    finding: dict,
    dry_run: bool,
    existing_labels: set[str],
    dropped_labels: set[str],
) -> tuple[str, str]:
    raw_title = finding["title"]
    # Strip a leading [<review_name>] prefix if the report already added one;
    # we re-add it canonically below.
    raw_title = re.sub(rf"^\[{re.escape(review_name)}\]\s*", "", raw_title, flags=re.IGNORECASE)
    title = f"[{review_name}] {raw_title}"

    body = build_body(review_name, finding)
    labels = ["auto-filed", f"review:{review_name}"]
    suggested = finding.get("suggested_labels") or ""
    # Strip wrapping backticks/quotes — some reports wrap the whole list.
    suggested = suggested.strip().strip("`").strip("'").strip('"')
    for lbl in re.split(r",\s*", suggested):
        lbl = lbl.strip().strip("`")
        if not lbl or lbl in labels:
            continue
        if re.match(r"^P\d$", lbl, re.IGNORECASE):
            continue  # Priority labels owned by the Project field, not labels.
        if lbl in existing_labels:
            labels.append(lbl)
        else:
            dropped_labels.add(lbl)

    if dry_run:
        return ("dry-run", f"{title}\n    labels={labels}")

    with tempfile.NamedTemporaryFile("w", suffix=".md", delete=False) as f:
        f.write(body)
        body_file = f.name
    try:
        result = subprocess.run(
            ["gh", "issue", "create",
             "--title", title,
             "--body-file", body_file,
             "--label", ",".join(labels)],
            capture_output=True, text=True,
        )
        if result.returncode != 0:
            return ("error", f"{title}\n    {result.stderr.strip()}")
        return ("created", f"{title}\n    {result.stdout.strip()}")
    finally:
        os.unlink(body_file)


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--apply", action="store_true",
                        help="Actually create issues (default: dry-run).")
    parser.add_argument("--severities", default="Critical",
                        help="Comma-separated severities to file. "
                             "Default: Critical. Options: Critical,High,Medium.")
    args = parser.parse_args()

    severities = {s.strip() for s in args.severities.split(",") if s.strip()}
    dry_run = not args.apply

    existing: dict[str, int] = {}
    existing_titles: dict[str, int] = {}
    existing_labels: set[str] = set()
    print("Fetching existing labels...")
    existing_labels = fetch_existing_labels()
    print(f"  found {len(existing_labels)} labels")
    print("Fetching existing auto-filed issues...")
    existing, existing_titles = fetch_existing_issues()
    print(f"  found {len(existing)} fingerprints, {len(existing_titles)} titles")

    by_review: dict[str, list[dict]] = {}
    for report in sorted(REPORTS_DIR.glob("*_review.md")):
        review_name = parse_review_name(report)
        text = report.read_text()
        section = extract_issues_section(text)
        if not section:
            continue
        all_findings = parse_findings(section)
        kept = [f for f in all_findings if f["severity"] in severities]
        if kept:
            by_review[review_name] = kept

    total = sum(len(f) for f in by_review.values())
    print(f"\nFiltering severities: {sorted(severities)}")
    print(f"Parsed {total} findings across {len(by_review)} reviews:")
    for name, findings in by_review.items():
        print(f"  {name}: {len(findings)}")

    print(f"\n{'DRY RUN' if dry_run else 'APPLY'} mode\n")

    filed, deduped, errored = [], [], []
    dropped_labels: set[str] = set()
    for review_name, findings in by_review.items():
        for finding in findings:
            fp = finding.get("fingerprint")
            if not fp:
                errored.append((review_name, finding["title"], "missing fingerprint"))
                continue
            if fp in existing:
                deduped.append((review_name, finding["title"], existing[fp], "fingerprint"))
                continue
            canonical_title = f"[{review_name}] {finding['title']}"
            title_key = _normalize_title(canonical_title)
            if title_key in existing_titles:
                deduped.append(
                    (review_name, finding["title"], existing_titles[title_key], "title")
                )
                continue
            status, info = create_issue(
                review_name, finding, dry_run, existing_labels, dropped_labels,
            )
            if status in ("created", "dry-run"):
                filed.append((review_name, info))
            else:
                errored.append((review_name, finding["title"], info))

    print("=== Summary ===")
    print(f"{'Would file' if dry_run else 'Filed'}: {len(filed)}")
    print(f"Deduped:    {len(deduped)}")
    print(f"Errored:    {len(errored)}")

    if filed:
        print("\nFiled:" if not dry_run else "\nWould file:")
        for review, info in filed:
            print(f"  [{review}] {info}")
    if deduped:
        print("\nDeduped:")
        for review, title, num, how in deduped:
            print(f"  [{review}] #{num} ({how}): {title}")
    if errored:
        print("\nErrored:")
        for review, title, err in errored:
            print(f"  [{review}] {title}\n    {err}")
    if dropped_labels:
        print(f"\nDropped {len(dropped_labels)} unknown suggested labels:")
        for lbl in sorted(dropped_labels):
            print(f"  {lbl}")
        print("(Create them with `gh label create <name>` if you want them on future runs.)")

    return 0 if not errored else 1


if __name__ == "__main__":
    sys.exit(main())
