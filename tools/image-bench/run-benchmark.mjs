#!/usr/bin/env node

import fs from 'node:fs/promises';
import path from 'node:path';
import process from 'node:process';

const API_BASE = process.env.OPENAI_BASE_URL || 'https://api.openai.com/v1';
const API_KEY = process.env.OPENAI_API_KEY;

if (!API_KEY) {
  console.error('ERROR: OPENAI_API_KEY is not set in this terminal session.');
  process.exit(1);
}

const args = parseArgs(process.argv.slice(2));
const now = new Date();
const stamp = now.toISOString().replace(/[:.]/g, '-');
const rootDir = path.resolve(path.dirname(new URL(import.meta.url).pathname));
const outDir = path.join(rootDir, 'outputs', stamp);
await fs.mkdir(outDir, { recursive: true });

const cfg = {
  imageModel: args.imageModel || 'gpt-image-2',
  judgeModel: args.judgeModel || 'gpt-4.1-mini',
  size: args.size || '1024x1024',
  quality: args.quality || 'high',
  speedTotal: Number(args.speedTotal || 20),
  speedConcurrency: Number(args.speedConcurrency || 4),
  qualityRepeats: Number(args.qualityRepeats || 1),
  timeoutMs: Number(args.timeoutMs || 180000),
  speedPromptsPath: args.speedPrompts || path.join(rootDir, 'prompts.speed.json'),
  qualityPromptsPath: args.qualityPrompts || path.join(rootDir, 'prompts.quality.json')
};

console.log(`Running benchmark with imageModel=${cfg.imageModel}, size=${cfg.size}, quality=${cfg.quality}`);
console.log(`Speed run: total=${cfg.speedTotal}, concurrency=${cfg.speedConcurrency}`);
console.log(`Quality run: repeats=${cfg.qualityRepeats} per prompt`);

const speedPrompts = JSON.parse(await fs.readFile(cfg.speedPromptsPath, 'utf8'));
const qualityPrompts = JSON.parse(await fs.readFile(cfg.qualityPromptsPath, 'utf8'));

const speedResults = await runSpeedBenchmark(cfg, speedPrompts, outDir);
const qualityResults = await runQualityBenchmark(cfg, qualityPrompts, outDir);
const summary = buildSummary(cfg, speedResults, qualityResults);

await fs.writeFile(path.join(outDir, 'speed_results.json'), JSON.stringify(speedResults, null, 2));
await fs.writeFile(path.join(outDir, 'quality_results.json'), JSON.stringify(qualityResults, null, 2));
await fs.writeFile(path.join(outDir, 'summary.json'), JSON.stringify(summary, null, 2));
await fs.writeFile(path.join(outDir, 'REPORT.md'), renderReport(summary, speedResults, qualityResults));

console.log(`Benchmark complete. Report: ${path.join(outDir, 'REPORT.md')}`);

function parseArgs(rawArgs) {
  const out = {};
  for (let i = 0; i < rawArgs.length; i += 1) {
    const k = rawArgs[i];
    if (!k.startsWith('--')) continue;
    const key = k.slice(2);
    const v = rawArgs[i + 1];
    if (!v || v.startsWith('--')) {
      out[key] = 'true';
      continue;
    }
    out[key] = v;
    i += 1;
  }
  return out;
}

async function runSpeedBenchmark(cfg, prompts, outDir) {
  const jobs = [];
  for (let i = 0; i < cfg.speedTotal; i += 1) {
    const prompt = prompts[i % prompts.length];
    jobs.push({ idx: i + 1, prompt });
  }

  const results = await runWithConcurrency(jobs, cfg.speedConcurrency, async (job) => {
    const startedAt = Date.now();
    console.log(`[speed] start #${job.idx}/${jobs.length}`);
    try {
      const image = await generateImage({
        model: cfg.imageModel,
        prompt: job.prompt,
        size: cfg.size,
        quality: cfg.quality,
        timeoutMs: cfg.timeoutMs
      });
      const latencyMs = Date.now() - startedAt;
      const fileName = `speed_${String(job.idx).padStart(3, '0')}.png`;
      await fs.writeFile(path.join(outDir, fileName), Buffer.from(image.b64, 'base64'));
      console.log(`[speed] done  #${job.idx}/${jobs.length} ${latencyMs}ms`);
      return {
        idx: job.idx,
        success: true,
        latencyMs,
        fileName,
        status: image.status,
        requestId: image.requestId,
        error: null
      };
    } catch (err) {
      console.log(`[speed] fail  #${job.idx}/${jobs.length} ${Date.now() - startedAt}ms ${err.message || String(err)}`);
      return {
        idx: job.idx,
        success: false,
        latencyMs: Date.now() - startedAt,
        fileName: null,
        status: err.status || 0,
        requestId: err.requestId || null,
        error: err.message || String(err)
      };
    }
  });

  return results.sort((a, b) => a.idx - b.idx);
}

async function runQualityBenchmark(cfg, prompts, outDir) {
  if (cfg.qualityRepeats <= 0) {
    console.log('[quality] skipped because qualityRepeats <= 0');
    return [];
  }

  const all = [];
  let seq = 1;
  const total = prompts.length * cfg.qualityRepeats;
  for (const p of prompts) {
    for (let i = 0; i < cfg.qualityRepeats; i += 1) {
      const id = `${p.id}_r${i + 1}`;
      const startedAt = Date.now();
      console.log(`[quality] start #${seq}/${total} ${id}`);
      try {
        const image = await generateImage({
          model: cfg.imageModel,
          prompt: p.prompt,
          size: cfg.size,
          quality: cfg.quality,
          timeoutMs: cfg.timeoutMs
        });

        const imageFile = `quality_${String(seq).padStart(3, '0')}_${id}.png`;
        await fs.writeFile(path.join(outDir, imageFile), Buffer.from(image.b64, 'base64'));
        const judged = await judgeImage({
          judgeModel: cfg.judgeModel,
          prompt: p.prompt,
          imageB64: image.b64,
          timeoutMs: cfg.timeoutMs
        });

        all.push({
          seq,
          id,
          prompt: p.prompt,
          success: true,
          latencyMs: Date.now() - startedAt,
          imageFile,
          requestId: image.requestId,
          status: image.status,
          judge: judged,
          error: null
        });
        console.log(`[quality] done  #${seq}/${total} ${id} ${Date.now() - startedAt}ms`);
      } catch (err) {
        all.push({
          seq,
          id,
          prompt: p.prompt,
          success: false,
          latencyMs: Date.now() - startedAt,
          imageFile: null,
          requestId: err.requestId || null,
          status: err.status || 0,
          judge: null,
          error: err.message || String(err)
        });
        console.log(`[quality] fail  #${seq}/${total} ${id} ${Date.now() - startedAt}ms ${err.message || String(err)}`);
      }
      seq += 1;
    }
  }
  return all;
}

async function generateImage({ model, prompt, size, quality, timeoutMs }) {
  const body = { model, prompt, size, quality };
  const res = await fetchWithTimeout(`${API_BASE}/images/generations`, {
    method: 'POST',
    headers: {
      Authorization: `Bearer ${API_KEY}`,
      'Content-Type': 'application/json'
    },
    body: JSON.stringify(body)
  }, timeoutMs);

  const requestId = res.headers.get('x-request-id') || null;
  const text = await res.text();
  let json;
  try {
    json = JSON.parse(text);
  } catch {
    throw enrichError(new Error(`Invalid JSON from images endpoint: ${text.slice(0, 300)}`), res.status, requestId);
  }

  if (!res.ok) {
    const msg = json?.error?.message || text || `HTTP ${res.status}`;
    throw enrichError(new Error(msg), res.status, requestId);
  }

  const b64 = json?.data?.[0]?.b64_json;
  if (!b64) {
    throw enrichError(new Error('No data[0].b64_json in images response'), res.status, requestId);
  }

  return { b64, status: res.status, requestId };
}

async function judgeImage({ judgeModel, prompt, imageB64, timeoutMs }) {
  const rubric = [
    'Return strict JSON only with keys: instruction_following, aesthetics, text_rendering, structure_integrity, overall, reason.',
    'Each score must be integer 1-5.',
    'overall should be the average tendency of the four dimensions.',
    'text_rendering should assess legibility/relevance of text if prompt asks text; otherwise score neutral by quality.'
  ].join(' ');

  const body = {
    model: judgeModel,
    response_format: { type: 'json_object' },
    messages: [
      {
        role: 'system',
        content: 'You are a strict image quality evaluator.'
      },
      {
        role: 'user',
        content: [
          { type: 'text', text: `Prompt: ${prompt}\n\n${rubric}` },
          { type: 'image_url', image_url: { url: `data:image/png;base64,${imageB64}` } }
        ]
      }
    ]
  };

  const res = await fetchWithTimeout(`${API_BASE}/chat/completions`, {
    method: 'POST',
    headers: {
      Authorization: `Bearer ${API_KEY}`,
      'Content-Type': 'application/json'
    },
    body: JSON.stringify(body)
  }, timeoutMs);

  const text = await res.text();
  let json;
  try {
    json = JSON.parse(text);
  } catch {
    throw new Error(`Invalid JSON from judge endpoint: ${text.slice(0, 300)}`);
  }

  if (!res.ok) {
    const msg = json?.error?.message || text || `HTTP ${res.status}`;
    throw new Error(`Judge request failed: ${msg}`);
  }

  const content = json?.choices?.[0]?.message?.content;
  if (!content) throw new Error('Judge returned empty content');

  let parsed;
  try {
    parsed = JSON.parse(content);
  } catch {
    throw new Error(`Judge output is not JSON: ${content.slice(0, 300)}`);
  }

  return normalizeJudgeScore(parsed);
}

function normalizeJudgeScore(raw) {
  const keys = ['instruction_following', 'aesthetics', 'text_rendering', 'structure_integrity', 'overall'];
  const out = { reason: String(raw.reason || '') };
  for (const k of keys) {
    const n = Number(raw[k]);
    out[k] = Number.isFinite(n) ? clamp(Math.round(n), 1, 5) : 1;
  }
  return out;
}

function clamp(v, min, max) {
  return Math.max(min, Math.min(max, v));
}

function buildSummary(cfg, speed, quality) {
  const speedSuccess = speed.filter((x) => x.success);
  const speedFailed = speed.filter((x) => !x.success);
  const latencies = speedSuccess.map((x) => x.latencyMs).sort((a, b) => a - b);

  const qualitySuccess = quality.filter((x) => x.success && x.judge);
  const qualityFailed = quality.filter((x) => !x.success);

  const scoreAvg = averageByKeys(qualitySuccess.map((x) => x.judge), [
    'instruction_following',
    'aesthetics',
    'text_rendering',
    'structure_integrity',
    'overall'
  ]);

  const totalDurationSec = (speed.reduce((acc, x) => acc + x.latencyMs, 0) / 1000) / Math.max(1, cfg.speedConcurrency);

  return {
    generatedAt: new Date().toISOString(),
    config: cfg,
    speed: {
      total: speed.length,
      success: speedSuccess.length,
      failed: speedFailed.length,
      successRate: pct(speedSuccess.length, speed.length),
      p50Ms: percentile(latencies, 50),
      p90Ms: percentile(latencies, 90),
      p95Ms: percentile(latencies, 95),
      p99Ms: percentile(latencies, 99),
      avgMs: average(latencies),
      throughputPerMin: totalDurationSec > 0 ? Number(((speedSuccess.length / totalDurationSec) * 60).toFixed(2)) : 0
    },
    quality: {
      total: quality.length,
      success: qualitySuccess.length,
      failed: qualityFailed.length,
      successRate: pct(qualitySuccess.length, quality.length),
      avgScores: scoreAvg
    }
  };
}

function renderReport(summary, speed, quality) {
  const lines = [];
  lines.push('# GPT Image Benchmark Report');
  lines.push('');
  lines.push(`- Generated At: ${summary.generatedAt}`);
  lines.push(`- Image Model: ${summary.config.imageModel}`);
  lines.push(`- Judge Model: ${summary.config.judgeModel}`);
  lines.push(`- Size: ${summary.config.size}`);
  lines.push(`- Quality Param: ${summary.config.quality}`);
  lines.push('');

  lines.push('## Speed');
  lines.push(`- Total: ${summary.speed.total}`);
  lines.push(`- Success: ${summary.speed.success}`);
  lines.push(`- Failed: ${summary.speed.failed}`);
  lines.push(`- Success Rate: ${summary.speed.successRate}%`);
  lines.push(`- Avg: ${summary.speed.avgMs} ms`);
  lines.push(`- P50: ${summary.speed.p50Ms} ms`);
  lines.push(`- P90: ${summary.speed.p90Ms} ms`);
  lines.push(`- P95: ${summary.speed.p95Ms} ms`);
  lines.push(`- P99: ${summary.speed.p99Ms} ms`);
  lines.push(`- Throughput: ${summary.speed.throughputPerMin} images/min`);
  lines.push('');

  lines.push('### Failed Speed Calls');
  const speedFail = speed.filter((x) => !x.success);
  if (speedFail.length === 0) {
    lines.push('- None');
  } else {
    for (const f of speedFail.slice(0, 20)) {
      lines.push(`- #${f.idx}: status=${f.status}, error=${sanitize(f.error)}`);
    }
  }
  lines.push('');

  lines.push('## Quality');
  lines.push(`- Total: ${summary.quality.total}`);
  lines.push(`- Success: ${summary.quality.success}`);
  lines.push(`- Failed: ${summary.quality.failed}`);
  lines.push(`- Success Rate: ${summary.quality.successRate}%`);
  lines.push(`- Avg instruction_following: ${summary.quality.avgScores.instruction_following}`);
  lines.push(`- Avg aesthetics: ${summary.quality.avgScores.aesthetics}`);
  lines.push(`- Avg text_rendering: ${summary.quality.avgScores.text_rendering}`);
  lines.push(`- Avg structure_integrity: ${summary.quality.avgScores.structure_integrity}`);
  lines.push(`- Avg overall: ${summary.quality.avgScores.overall}`);
  lines.push('');

  const qOK = quality.filter((x) => x.success && x.judge);
  const top = [...qOK].sort((a, b) => b.judge.overall - a.judge.overall).slice(0, 5);
  const low = [...qOK].sort((a, b) => a.judge.overall - b.judge.overall).slice(0, 5);

  lines.push('### Top Samples');
  if (top.length === 0) lines.push('- None');
  for (const t of top) {
    lines.push(`- ${t.id}: overall=${t.judge.overall}, aesthetics=${t.judge.aesthetics}, file=${t.imageFile}`);
  }
  lines.push('');

  lines.push('### Low Samples');
  if (low.length === 0) lines.push('- None');
  for (const t of low) {
    lines.push(`- ${t.id}: overall=${t.judge.overall}, aesthetics=${t.judge.aesthetics}, file=${t.imageFile}`);
  }
  lines.push('');

  lines.push('### Failed Quality Calls');
  const qFail = quality.filter((x) => !x.success);
  if (qFail.length === 0) {
    lines.push('- None');
  } else {
    for (const f of qFail.slice(0, 20)) {
      lines.push(`- ${f.id}: status=${f.status}, error=${sanitize(f.error)}`);
    }
  }

  return `${lines.join('\n')}\n`;
}

async function runWithConcurrency(items, concurrency, worker) {
  const results = new Array(items.length);
  let cursor = 0;

  async function runner() {
    while (true) {
      const i = cursor;
      cursor += 1;
      if (i >= items.length) return;
      results[i] = await worker(items[i]);
    }
  }

  const n = Math.max(1, concurrency);
  await Promise.all(Array.from({ length: n }, () => runner()));
  return results;
}

async function fetchWithTimeout(url, opts, timeoutMs) {
  const ctl = new AbortController();
  const timer = setTimeout(() => ctl.abort(), timeoutMs);
  try {
    return await fetch(url, { ...opts, signal: ctl.signal });
  } catch (err) {
    if (err && err.name === 'AbortError') {
      throw new Error(`Request timed out after ${timeoutMs} ms`);
    }
    throw err;
  } finally {
    clearTimeout(timer);
  }
}

function enrichError(err, status, requestId) {
  err.status = status;
  err.requestId = requestId;
  return err;
}

function percentile(sortedAsc, p) {
  if (!sortedAsc.length) return 0;
  const idx = Math.ceil((p / 100) * sortedAsc.length) - 1;
  return sortedAsc[Math.max(0, Math.min(sortedAsc.length - 1, idx))];
}

function average(arr) {
  if (!arr.length) return 0;
  return Number((arr.reduce((a, b) => a + b, 0) / arr.length).toFixed(2));
}

function averageByKeys(list, keys) {
  const out = {};
  for (const k of keys) {
    out[k] = average(list.map((x) => Number(x[k] || 0)));
  }
  return out;
}

function pct(a, b) {
  if (!b) return 0;
  return Number(((a / b) * 100).toFixed(2));
}

function sanitize(text) {
  return String(text || '').replace(/\s+/g, ' ').slice(0, 300);
}
