#!/bin/bash
# Generate all code review reports in parallel using Claude Code

set -e

REVIEWS_DIR="docs/reviews"
REPORTS_DIR="docs/reviews/reports"

# List of all review prompts (without architecture - run separately if needed)
REVIEWS=(
  "security"
  "dependency"
  "performance"
  "testing"
  "error_handling"
  "accessibility"
  "tech_debt"
  "api_design"
  "observability"
)

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

echo "Starting parallel report generation for ${#REVIEWS[@]} reviews..."
echo ""

# Create an array to hold PIDs
declare -a PIDS

# Start all reviews in parallel
for review in "${REVIEWS[@]}"; do
  prompt_file="${REVIEWS_DIR}/${review}_review_prompt.md"
  report_file="${REPORTS_DIR}/${review}_review.md"

  if [ ! -f "$prompt_file" ]; then
    echo -e "${RED}Prompt not found: $prompt_file${NC}"
    continue
  fi

  echo -e "${YELLOW}Starting: ${review} review${NC}"

  # Run claude in background, redirecting output to a log file
  (
    claude --print \
      --dangerously-skip-permissions \
      "Follow the instructions in ${prompt_file} to generate the ${review} review report. Write the report to ${report_file}." \
      > "/tmp/claude_${review}_review.log" 2>&1

    if [ $? -eq 0 ]; then
      echo -e "${GREEN}Completed: ${review} review${NC}"
    else
      echo -e "${RED}Failed: ${review} review (check /tmp/claude_${review}_review.log)${NC}"
    fi
  ) &

  PIDS+=($!)
done

echo ""
echo "All reviews started. Waiting for completion..."
echo ""

# Wait for all background processes
FAILED=0
for pid in "${PIDS[@]}"; do
  wait $pid || ((FAILED++))
done

echo ""
if [ $FAILED -eq 0 ]; then
  echo -e "${GREEN}All ${#REVIEWS[@]} reviews completed successfully!${NC}"
else
  echo -e "${RED}${FAILED} review(s) failed. Check logs in /tmp/claude_*_review.log${NC}"
fi

echo ""
echo "Reports available in: ${REPORTS_DIR}/"
ls -la "${REPORTS_DIR}/"
