const { execFileSync } = require("node:child_process");
const crypto = require("node:crypto");
const fs = require("node:fs");
const path = require("node:path");

const model = process.env.SUB2API_TEST_MODEL || "claude-opus-4-6";
const localUrl = process.env.SUB2API_LOCAL_URL || "http://127.0.0.1:8080/v1/messages";
const slaMs = Number(process.env.SUB2API_SLA_MS || 15000);
const boostedUserConcurrency = Number(process.env.SUB2API_TEST_USER_CONCURRENCY || 1500);
const stages = (process.env.SUB2API_STAGES || "10,50,100,250,500,1000")
  .split(",")
  .map((v) => Number(v.trim()))
  .filter((v) => Number.isFinite(v) && v > 0);
const directStages = new Set(
  (process.env.SUB2API_DIRECT_STAGES || "10,100,500,1000")
    .split(",")
    .map((v) => Number(v.trim()))
    .filter((v) => Number.isFinite(v) && v > 0),
);
const targets = (process.env.SUB2API_TARGETS || "local,upstream")
  .split(",")
  .map((v) => v.trim().toLowerCase())
  .filter((v) => v === "local" || v === "upstream");
const perStageTotal = Number(process.env.SUB2API_STAGE_TOTAL || 0);

function sh(cmd, args, opts = {}) {
  return execFileSync(cmd, args, {
    encoding: "utf8",
    stdio: ["ignore", "pipe", "pipe"],
    ...opts,
  }).trim();
}

function psql(sql) {
  return sh("docker", [
    "exec",
    "sub2api-postgres-dev",
    "psql",
    "-U",
    "sub2api",
    "-d",
    "sub2api",
    "-tA",
    "-c",
    sql,
  ]);
}

function redis(args) {
  return sh("docker", ["exec", "sub2api-redis-dev", "redis-cli", ...args]);
}

function parseOneInt(text, fallback = 0) {
  const n = Number(String(text).trim().split(/\r?\n/).filter(Boolean).at(-1));
  return Number.isFinite(n) ? n : fallback;
}

function getKeys() {
  const localKey = psql("SELECT key FROM api_keys WHERE id=2;");
  const cred = psql(
    "SELECT credentials->>'api_key' || E'\\t' || COALESCE(credentials->>'base_url','') FROM accounts WHERE id=33;",
  );
  const [upstreamKey, upstreamBaseRaw] = cred.split("\t");
  return {
    localKey,
    upstreamKey,
    upstreamBase: (upstreamBaseRaw || "https://cn.meai.cloud").replace(/\/+$/, ""),
  };
}

function authCacheKey(key) {
  return crypto.createHash("sha256").update(key).digest("hex");
}

function clearConcurrencyKeys() {
  try {
    sh("docker", [
      "exec",
      "sub2api-redis-dev",
      "sh",
      "-lc",
      "unset REDISCLI_AUTH; redis-cli --scan --pattern 'concurrency:*' | xargs -r redis-cli del >/dev/null",
    ]);
  } catch (err) {
    console.error("WARN clear concurrency keys failed:", String(err.message || err));
  }
}

function invalidateAuthCache(localKey) {
  const key = authCacheKey(localKey);
  try {
    redis(["DEL", `apikey:auth:${key}`]);
    redis(["PUBLISH", "auth:cache:invalidate", key]);
  } catch (err) {
    console.error("WARN auth cache invalidation failed:", String(err.message || err));
  }
}

function body(i, stream = false) {
  return {
    model,
    max_tokens: 1,
    stream,
    messages: [
      {
        role: "user",
        content: `Return exactly one digit. Request ${i}.`,
      },
    ],
  };
}

function headers(target, keys, reqId) {
  if (target === "local") {
    return {
      "content-type": "application/json",
      "authorization": `Bearer ${keys.localKey}`,
      "x-api-key": keys.localKey,
      "anthropic-version": "2023-06-01",
      "x-client-request-id": reqId,
      "user-agent": "sub2api-enterprise-loadtest/local",
    };
  }
  return {
    "content-type": "application/json",
    "x-api-key": keys.upstreamKey,
    "anthropic-version": "2023-06-01",
    "x-client-request-id": reqId,
    "user-agent": "sub2api-enterprise-loadtest/direct",
  };
}

function summarize(name, stage, target, results) {
  const ok = results.filter((r) => r.ok);
  const slaOk = results.filter((r) => r.ok && r.latency_ms <= slaMs);
  const fail = results.filter((r) => !r.ok);
  const slow = results.filter((r) => r.ok && r.latency_ms > slaMs);
  const lat = ok.map((r) => r.latency_ms).sort((a, b) => a - b);
  const pct = (p) => {
    if (!lat.length) return null;
    return lat[Math.min(lat.length - 1, Math.max(0, Math.ceil((p / 100) * lat.length) - 1))];
  };
  const errors = {};
  for (const r of fail) {
    const k = [r.status ? `HTTP_${r.status}` : r.error_name || "ERROR", r.error_code || r.error_summary || r.error_message || ""]
      .join(" ")
      .replace(/\s+/g, " ")
      .slice(0, 220);
    errors[k] = (errors[k] || 0) + 1;
  }
  return {
    name,
    stage_concurrency: stage,
    target,
    total: results.length,
    success: ok.length,
    failed: fail.length,
    slow_over_sla: slow.length,
    success_rate_pct: Number(((ok.length / results.length) * 100).toFixed(2)),
    sla_success_rate_pct: Number(((slaOk.length / results.length) * 100).toFixed(2)),
    latency_ms: lat.length
      ? {
          min: lat[0],
          p50: pct(50),
          p90: pct(90),
          p95: pct(95),
          p99: pct(99),
          max: lat.at(-1),
        }
      : null,
    errors: Object.entries(errors)
      .sort((a, b) => b[1] - a[1])
      .slice(0, 8)
      .map(([message, count]) => ({ count, message })),
  };
}

async function one(target, keys, i, stage) {
  const reqId = `${target}-c${stage}-${i}-${crypto.randomUUID()}`;
  const url = target === "local" ? localUrl : `${keys.upstreamBase}/v1/messages?beta=true`;
  const ac = new AbortController();
  const timer = setTimeout(() => ac.abort(new Error(`client_sla_timeout_${slaMs}ms`)), slaMs);
  const started = Date.now();
  try {
    const res = await fetch(url, {
      method: "POST",
      headers: headers(target, keys, reqId),
      body: JSON.stringify(body(i)),
      signal: ac.signal,
    });
    const latency = Date.now() - started;
    const text = await res.text();
    let json = null;
    try {
      json = JSON.parse(text);
    } catch {}
    if (!res.ok) {
      const errObj = json?.error || json;
      return {
        ok: false,
        status: res.status,
        latency_ms: latency,
        request_id: reqId,
        error_code: errObj?.code || errObj?.type || "",
        error_summary: errObj?.message || text.slice(0, 240),
      };
    }
    const outputText = Array.isArray(json?.content)
      ? json.content.map((block) => block?.text || "").join("")
      : "";
    return {
      ok: true,
      status: res.status,
      latency_ms: latency,
      request_id: reqId,
      output_tokens: json?.usage?.output_tokens,
      cache_read: json?.usage?.cache_read_input_tokens,
      text_len: outputText.length,
    };
  } catch (err) {
    return {
      ok: false,
      status: 0,
      latency_ms: Date.now() - started,
      request_id: reqId,
      error_name: err?.name || "Error",
      error_message: String(err?.message || err),
    };
  } finally {
    clearTimeout(timer);
  }
}

async function runStage(target, keys, stage) {
  const total = perStageTotal > 0 ? perStageTotal : stage;
  const results = new Array(total);
  let next = 0;
  async function worker() {
    while (true) {
      const idx = next++;
      if (idx >= total) return;
      results[idx] = await one(target, keys, idx + 1, stage);
    }
  }
  const started = new Date();
  await Promise.all(Array.from({ length: Math.min(stage, total) }, worker));
  const ended = new Date();
  const summary = summarize(`${target}_c${stage}`, stage, target, results);
  summary.started_at = started.toISOString();
  summary.ended_at = ended.toISOString();
  console.log(JSON.stringify(summary));
  return { summary, results };
}

function dockerStats() {
  try {
    return sh("docker", [
      "stats",
      "--no-stream",
      "--format",
      "{{.Name}} {{.CPUPerc}} {{.MemUsage}} {{.NetIO}}",
      "sub2api-dev",
      "sub2api-postgres-dev",
      "sub2api-redis-dev",
    ]);
  } catch (err) {
    return `stats failed: ${err.message || err}`;
  }
}

async function main() {
  const keys = getKeys();
  const originalConcurrency = parseOneInt(psql("SELECT concurrency FROM users WHERE id=1;"), 5);
  const startedAt = new Date().toISOString();
  const reportDir = path.resolve("tmp");
  fs.mkdirSync(reportDir, { recursive: true });
  const reportPath = path.join(reportDir, `default-loadtest-${startedAt.replace(/[:.]/g, "-")}.jsonl`);
  console.log(
    JSON.stringify({
      event: "start",
      started_at: startedAt,
      model,
      local_url: localUrl,
      upstream_host: new URL(keys.upstreamBase).host,
      sla_ms: slaMs,
      stages,
      direct_stages: Array.from(directStages).sort((a, b) => a - b),
      targets,
      original_user_concurrency: originalConcurrency,
      boosted_user_concurrency: boostedUserConcurrency,
      report_path: reportPath,
    }),
  );
  try {
    clearConcurrencyKeys();
    psql(`UPDATE users SET concurrency=${boostedUserConcurrency}, updated_at=now() WHERE id=1;`);
    invalidateAuthCache(keys.localKey);
    await new Promise((resolve) => setTimeout(resolve, 1500));

    const allSummaries = [];
    for (const stage of stages) {
      let local = null;
      if (targets.includes("local")) {
        console.log(JSON.stringify({ event: "stage_begin", target: "local", stage, stats_before: dockerStats() }));
        local = await runStage("local", keys, stage);
        allSummaries.push(local.summary);
        fs.appendFileSync(reportPath, JSON.stringify({ summary: local.summary, sample_failures: local.results.filter((r) => !r.ok).slice(0, 10) }) + "\n");
      }

      const shouldDirectCompare =
        targets.includes("upstream") &&
        (directStages.has(stage) ||
        !targets.includes("local") ||
        local.summary.sla_success_rate_pct < 95 ||
        local.summary.failed > 0);
      if (shouldDirectCompare) {
        console.log(JSON.stringify({ event: "stage_begin", target: "upstream", stage, stats_before: dockerStats() }));
        const upstream = await runStage("upstream", keys, stage);
        allSummaries.push(upstream.summary);
        fs.appendFileSync(reportPath, JSON.stringify({ summary: upstream.summary, sample_failures: upstream.results.filter((r) => !r.ok).slice(0, 10) }) + "\n");
      }
    }
    console.log(JSON.stringify({ event: "done", summaries: allSummaries, stats_after: dockerStats(), report_path: reportPath }));
  } finally {
    psql(`UPDATE users SET concurrency=${originalConcurrency}, updated_at=now() WHERE id=1;`);
    invalidateAuthCache(keys.localKey);
    clearConcurrencyKeys();
    console.log(JSON.stringify({ event: "restored", user_id: 1, concurrency: originalConcurrency }));
  }
}

main().catch((err) => {
  console.error(JSON.stringify({ event: "fatal", error: String(err?.stack || err) }));
  process.exit(1);
});
