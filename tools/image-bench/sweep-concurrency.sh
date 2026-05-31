#!/usr/bin/env bash
set -euo pipefail

if [[ -z "${OPENAI_API_KEY:-}" ]]; then
  echo "ERROR: OPENAI_API_KEY is not set"
  exit 1
fi

DIR="$(cd "$(dirname "$0")" && pwd)"
TIMEOUT=${TIMEOUT_MS:-240000}
QUALITY=${QUALITY:-medium}
RESULT_FILE="$DIR/sweep-$(date +%Y%m%dT%H%M%S).txt"

echo "Starting concurrency sweep. Results → $RESULT_FILE"
echo "Model: gpt-image-2  Size: 1024x1024  Quality: $QUALITY" | tee "$RESULT_FILE"
echo "" | tee -a "$RESULT_FILE"

for CONC in 50 100 150 200; do
  TOTAL=$((CONC * 2))
  echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━" | tee -a "$RESULT_FILE"
  echo "concurrency=$CONC  total=$TOTAL" | tee -a "$RESULT_FILE"
  echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━" | tee -a "$RESULT_FILE"

  node "$DIR/run-benchmark.mjs" \
    --speedTotal "$TOTAL" \
    --speedConcurrency "$CONC" \
    --qualityRepeats 0 \
    --timeoutMs "$TIMEOUT" \
    --quality "$QUALITY" 2>&1 | tee -a "$RESULT_FILE"

  # extract key metrics from the latest report
  LATEST=$(ls -1 "$DIR/outputs" | tail -n 1)
  REPORT="$DIR/outputs/$LATEST/REPORT.md"
  echo "" | tee -a "$RESULT_FILE"
  echo "── Summary ──" | tee -a "$RESULT_FILE"
  grep -E 'Success Rate|Avg:|P50:|P90:|P95:|Throughput|Failed:' "$REPORT" | tee -a "$RESULT_FILE"
  echo "" | tee -a "$RESULT_FILE"
done

echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━" | tee -a "$RESULT_FILE"
echo "Sweep complete. Full results in: $RESULT_FILE" | tee -a "$RESULT_FILE"
