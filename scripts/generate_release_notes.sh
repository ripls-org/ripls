#!/bin/bash
# Converts markdown release notes to HTML and plain text using pandoc
# HTML files go to website/content/release-notes/ for web publishing
# Plain text files go alongside the markdown for app store changelogs

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(dirname "$SCRIPT_DIR")"

NOTES_DIR="$PROJECT_ROOT/docs/release/notes"
OUTPUT_DIR="$PROJECT_ROOT/website/content/release-notes"
TEMPLATE="$PROJECT_ROOT/website/templates/release-notes.html"

# Check for pandoc
if ! command -v pandoc &> /dev/null; then
    echo "Warning: pandoc not installed, skipping release notes generation"
    exit 0
fi

# Create output directory
mkdir -p "$OUTPUT_DIR"

# Find all markdown release notes, sorted by version
shopt -s nullglob
MD_FILES=("$NOTES_DIR"/*.md)

if [ ${#MD_FILES[@]} -eq 0 ]; then
    echo "No release notes found in $NOTES_DIR"
    exit 0
fi

# Sort versions for previous-version linking
SORTED_VERSIONS=()
for md_file in "${MD_FILES[@]}"; do
    SORTED_VERSIONS+=("$(basename "$md_file" .md)")
done
# Read loop rather than `mapfile -t`: the shebang is /bin/bash, which on macOS
# is bash 3.2, and mapfile is a bash 4 builtin. (Also avoids the unquoted
# command-substitution word-splitting that SC2207 flags.)
SORTED_VERSIONS_TMP=()
while IFS= read -r _version; do
    SORTED_VERSIONS_TMP+=("$_version")
done < <(printf '%s\n' "${SORTED_VERSIONS[@]}" | sort -V)
SORTED_VERSIONS=("${SORTED_VERSIONS_TMP[@]}")
unset SORTED_VERSIONS_TMP

# Get release date from git tag (falls back to today's date)
get_release_date() {
    local ver="$1"
    local tag_date
    tag_date=$(git tag -l "v${ver}" --format='%(creatordate:short)' 2>/dev/null)
    if [ -n "$tag_date" ]; then
        echo "$tag_date"
    else
        date '+%Y-%m-%d'
    fi
}

# Format a date as "Month Day, Year" (e.g. "February 5, 2026")
format_date() {
    local iso_date="$1"
    if [ "$iso_date" = "unreleased" ]; then
        echo "unreleased"
        return
    fi
    # macOS date uses -j -f; GNU date uses -d
    if date -j -f '%Y-%m-%d' "$iso_date" '+%B %-d, %Y' 2>/dev/null; then
        return
    fi
    date -d "$iso_date" '+%B %-d, %Y' 2>/dev/null || echo "$iso_date"
}

for md_file in "${MD_FILES[@]}"; do
    filename=$(basename "$md_file" .md)
    html_file="$OUTPUT_DIR/${filename}.html"
    txt_file="$OUTPUT_DIR/${filename}.txt"

    echo "Converting $filename..."

    # Get release date for this version
    release_date=$(get_release_date "$filename")
    release_date_display=$(format_date "$release_date")

    # Find this version's index in sorted list
    current_idx=-1
    for i in "${!SORTED_VERSIONS[@]}"; do
        if [ "${SORTED_VERSIONS[$i]}" = "$filename" ]; then
            current_idx=$i
            break
        fi
    done

    # Build YAML metadata file with release date and previous releases
    metadata_file=$(mktemp)
    # Single quotes: expand at trap time, not now. This runs inside the
    # per-release loop, so each iteration replaces the previous trap.
    # shellcheck disable=SC2064
    trap 'rm -f "$metadata_file"' EXIT
    {
        echo "title: \"Ripls ${filename} Release Notes\""
        echo "release_date: \"${release_date_display}\""
        # Collect up to 5 previous versions (newest first)
        if [ "$current_idx" -gt 0 ]; then
            echo "prev_releases:"
            start_idx=$((current_idx - 5))
            if [ "$start_idx" -lt 0 ]; then
                start_idx=0
            fi
            # Build list in reverse order (most recent previous first)
            prev_idx=$((current_idx - 1))
            while [ "$prev_idx" -ge "$start_idx" ]; do
                prev_ver="${SORTED_VERSIONS[$prev_idx]}"
                prev_date=$(format_date "$(get_release_date "$prev_ver")")
                echo "  - version: \"${prev_ver}\""
                echo "    date: \"${prev_date}\""
                prev_idx=$((prev_idx - 1))
            done
        fi
    } > "$metadata_file"

    # Convert to HTML using template
    pandoc -f markdown -t html \
        --template="$TEMPLATE" \
        --metadata-file="$metadata_file" \
        -o "$html_file" \
        "$md_file"

    rm -f "$metadata_file"

    # Insert release date after the first </h1> tag
    sed -i.bak "1,/<\/h1>/s|</h1>|</h1>\n            <p class=\"release-date\">${release_date_display}</p>|" "$html_file"
    rm -f "${html_file}.bak"

    # Convert to plain text with URL after the title for app store changelogs
    # The store changelog links back to the hosted copy of these notes. Where
    # that is hosted is deployment-specific, so it comes from the environment
    # and the line is simply omitted when unset — this script runs as part of
    # `npm run generate`, so requiring the variable would break codegen for
    # everyone who does not publish release notes (#2953).
    plain_text=$(pandoc -f markdown -t plain --wrap=none "$md_file")

    # Insert URL link after the first line (title)
    title=$(echo "$plain_text" | head -n 1)
    rest=$(echo "$plain_text" | tail -n +2)
    {
        echo "$title"
        echo ""
        if [[ -n "${RELEASE_NOTES_BASE_URL:-}" ]]; then
            echo "Full release notes: ${RELEASE_NOTES_BASE_URL%/}/${filename}.html"
        fi
        echo "$rest"
    } > "$txt_file"

    echo "  -> $html_file"
    echo "  -> $txt_file"
done

echo "Done: ${#MD_FILES[@]} release notes converted"
