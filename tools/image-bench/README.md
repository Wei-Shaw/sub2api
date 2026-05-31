# GPT Image 2.0 Benchmark

This tool benchmarks generation speed and output quality for OpenAI image models.

## Quick Start

1. Set key in current terminal:

```bash
export OPENAI_API_KEY="<YOUR_KEY>"
```

2. Run benchmark:

```bash
node tools/image-bench/run-benchmark.mjs \
  --imageModel gpt-image-2 \
  --judgeModel gpt-4.1-mini \
  --speedTotal 20 \
  --speedConcurrency 4 \
  --qualityRepeats 1 \
  --size 1024x1024 \
  --quality high
```

3. Check report:

```bash
ls -1 tools/image-bench/outputs
cat tools/image-bench/outputs/<timestamp>/REPORT.md
```

## Tunable Flags

- `--imageModel`: image generation model (default `gpt-image-2`)
- `--judgeModel`: model for quality scoring (default `gpt-4.1-mini`)
- `--speedTotal`: number of speed requests (default `20`)
- `--speedConcurrency`: concurrent workers for speed run (default `4`)
- `--qualityRepeats`: repeats per quality prompt (default `1`)
- `--size`: image size, e.g. `1024x1024`
- `--quality`: generation quality parameter, e.g. `high`
- `--timeoutMs`: timeout in ms (default `180000`)
- `--speedPrompts`: custom speed prompt JSON path
- `--qualityPrompts`: custom quality prompt JSON path

## Output Files

Each run writes to `tools/image-bench/outputs/<timestamp>/`:

- `REPORT.md`: human-readable summary
- `summary.json`: aggregated metrics
- `speed_results.json`: per-request latency and error details
- `quality_results.json`: per-prompt scoring details
- `speed_*.png`, `quality_*.png`: generated sample images
