#!/usr/bin/env node

import { spawnSync } from 'node:child_process';
import { copyFileSync, existsSync } from 'node:fs';
import { basename, dirname, resolve } from 'node:path';
import { createRequire } from 'node:module';

const require = createRequire(import.meta.url);
const {
  hashApiKey,
  keyPreview,
  readStoredApiKey,
} = require('/Users/wujianxiang/CodeSpace/yui.web/lib/shop-api-key-crypto.js');

const DEFAULT_YUI_DB = '/Users/wujianxiang/CodeSpace/yui.web/data/shop.sqlite';
const DEFAULT_YUI_ENV = '/Users/wujianxiang/CodeSpace/yui.web/.env';
const DEFAULT_PG_CONTAINER = 'sub2api-postgres';
const DEFAULT_REDIS_CONTAINER = 'sub2api-redis';
const DEFAULT_QUOTA_DATE = '2026-06-18';
const DEFAULT_PERIOD_START = '2026-06-17T00:00:00+08:00';

const PLAN_TO_GROUP = new Map([
  ['sub_29_daily_19_usd', 'codex-pool-19-usd'],
  ['sub_39_daily_29_usd', 'codex-pool-29-usd'],
  ['sub_59_daily_49_usd', 'codex-pool-49-usd'],
]);

function parseArgs(argv) {
  const opts = {
    apply: false,
    backup: false,
    yuiDb: DEFAULT_YUI_DB,
    yuiEnv: DEFAULT_YUI_ENV,
    pgContainer: DEFAULT_PG_CONTAINER,
    redisContainer: DEFAULT_REDIS_CONTAINER,
    quotaDate: DEFAULT_QUOTA_DATE,
    periodStart: DEFAULT_PERIOD_START,
  };

  for (const arg of argv) {
    if (arg === '--apply') opts.apply = true;
    else if (arg === '--backup') opts.backup = true;
    else if (arg.startsWith('--yui-db=')) opts.yuiDb = arg.slice('--yui-db='.length);
    else if (arg.startsWith('--yui-env=')) opts.yuiEnv = arg.slice('--yui-env='.length);
    else if (arg.startsWith('--pg-container=')) opts.pgContainer = arg.slice('--pg-container='.length);
    else if (arg.startsWith('--redis-container=')) opts.redisContainer = arg.slice('--redis-container='.length);
    else if (arg.startsWith('--quota-date=')) opts.quotaDate = arg.slice('--quota-date='.length);
    else if (arg.startsWith('--period-start=')) opts.periodStart = arg.slice('--period-start='.length);
    else if (arg === '--help' || arg === '-h') {
      printHelp();
      process.exit(0);
    } else {
      throw new Error(`未知参数：${arg}`);
    }
  }

  return opts;
}

function printHelp() {
  console.log(`用法：
  node scripts/migrate-yuiweb-legacy-api-keys.mjs
  node scripts/migrate-yuiweb-legacy-api-keys.mjs --backup --apply

参数：
  --apply                 执行写入；默认只 dry-run
  --backup                apply 前创建 PostgreSQL / SQLite 备份
  --yui-db=PATH           yui.web SQLite 路径
  --yui-env=PATH          yui.web .env 路径
  --pg-container=NAME     Sub2API PostgreSQL 容器名
  --redis-container=NAME  Sub2API Redis 容器名
  --quota-date=YYYY-MM-DD 当日额度迁移日期
  --period-start=ISO      周期用量统计起点`);
}

function readEnv(path) {
  const text = run('env-read', 'sed', ['-n', '1,240p', path], { allowStdout: true });
  const env = {};
  for (const line of text.split('\n')) {
    const trimmed = line.trim();
    if (!trimmed || trimmed.startsWith('#')) continue;
    const index = trimmed.indexOf('=');
    if (index <= 0) continue;
    const key = trimmed.slice(0, index).trim();
    let value = trimmed.slice(index + 1).trim();
    if ((value.startsWith('"') && value.endsWith('"')) || (value.startsWith("'") && value.endsWith("'"))) {
      value = value.slice(1, -1);
    }
    env[key] = value;
  }
  return env;
}

function run(label, command, args, options = {}) {
  const result = spawnSync(command, args, {
    input: options.input,
    encoding: 'utf8',
    env: {
      ...process.env,
      PATH: `/Applications/Docker.app/Contents/Resources/bin:${process.env.PATH || ''}`,
    },
    maxBuffer: 1024 * 1024 * 64,
  });

  if (result.status !== 0) {
    const stdout = options.scrub ? options.scrub(result.stdout || '') : (result.stdout || '');
    const stderr = options.scrub ? options.scrub(result.stderr || '') : (result.stderr || '');
    throw new Error(`${label} 失败：exit=${result.status}\nstdout:\n${stdout}\nstderr:\n${stderr}`);
  }

  if (options.allowStdout) return result.stdout || '';
  return result.stdout || '';
}

function sqliteJson(dbPath, sql) {
  const output = run('sqlite3', 'sqlite3', ['-json', dbPath, sql], { allowStdout: true });
  return JSON.parse(output || '[]');
}

function dollarQuote(text) {
  const base = 'codex_json';
  let tag = base;
  while (text.includes(`$${tag}$`)) tag = `${tag}_x`;
  return `$${tag}$${text}$${tag}$`;
}

function psql(container, sql, scrub) {
  return run(
    'psql',
    'docker',
    ['exec', '-i', container, 'psql', '-U', 'sub2api', '-d', 'sub2api', '-v', 'ON_ERROR_STOP=1', '-t', '-A'],
    { input: sql, allowStdout: true, scrub },
  ).trim();
}

function parseLastJsonLine(output) {
  const lines = String(output || '').split('\n').map((line) => line.trim()).filter(Boolean);
  for (let index = lines.length - 1; index >= 0; index -= 1) {
    const line = lines[index];
    if (line.startsWith('{') || line.startsWith('[')) return JSON.parse(line);
  }
  throw new Error(`psql 输出中没有 JSON 行：\n${output}`);
}

function redisCli(container, commands, scrub) {
  return run(
    'redis-cli',
    'docker',
    ['exec', '-i', container, 'redis-cli', '--raw'],
    { input: commands, allowStdout: true, scrub },
  );
}

function maskPhone(phone) {
  const value = String(phone || '');
  if (value.length < 7) return '***';
  return `${value.slice(0, 3)}****${value.slice(-4)}`;
}

function scrubber(keys) {
  const values = keys.filter(Boolean).sort((a, b) => b.length - a.length);
  return (text) => {
    let result = String(text || '');
    for (const key of values) {
      result = result.split(key).join('[REDACTED_API_KEY]');
    }
    return result;
  };
}

function ensureInputFiles(opts) {
  for (const path of [opts.yuiDb, opts.yuiEnv]) {
    if (!existsSync(path)) throw new Error(`文件不存在：${path}`);
  }
}

function loadOrders(opts, secret) {
  const rows = sqliteJson(opts.yuiDb, `
select
  o.id as order_id,
  lower(trim(o.phone)) as phone,
  o.api_key as api_key,
  o.api_key_preview as api_key_preview,
  o.api_key_ciphertext as api_key_ciphertext,
  o.api_key_nonce as api_key_nonce,
  o.product_name as product_name,
  o.amount as amount,
  o.redeemed_at as redeemed_at,
  o.expires_at as order_expires_at,
  s.plan_id as sub_plan_id,
  s.status as sub_status,
  s.started_at as sub_started_at,
  s.expires_at as sub_expires_at,
  k.status as inventory_status,
  k.api_key_hash as inventory_hash,
  coalesce((
    select sum(c.daily_quota_deducted_usd_micros) / 1000000.0
    from api_usd_charge_records c
    where c.phone = o.phone
      and c.status = 'charged'
      and c.quota_date = '${opts.quotaDate.replaceAll("'", "''")}'
  ), 0.0) as daily_usage_usd,
  coalesce((
    select sum(c.daily_quota_deducted_usd_micros) / 1000000.0
    from api_usd_charge_records c
    where c.phone = o.phone
      and c.status = 'charged'
      and c.created_at >= coalesce(s.started_at, '${opts.periodStart.replaceAll("'", "''")}')
      and c.created_at < coalesce(s.expires_at, o.expires_at)
  ), 0.0) as period_usage_usd
from orders o
left join account_subscriptions s on s.phone = o.phone
left join api_keys k on k.order_id = o.id
order by o.redeemed_at;
`);

  return rows.map((row) => {
    const key = readStoredApiKey(row, secret).trim();
    const preview = keyPreview(key);
    const keyHash = hashApiKey(key);
    const hasActiveSub = row.sub_status === 'active' && PLAN_TO_GROUP.has(row.sub_plan_id);
    const groupName = hasActiveSub ? PLAN_TO_GROUP.get(row.sub_plan_id) : 'codex-pool-19-usd';
    const subSource = hasActiveSub ? 'active_subscription' : 'manual_order_subscription';
    const startsAt = hasActiveSub ? row.sub_started_at : row.redeemed_at;
    const expiresAt = hasActiveSub ? row.sub_expires_at : row.order_expires_at;

    return {
      order_id: row.order_id,
      phone: row.phone,
      phone_masked: maskPhone(row.phone),
      email: `${row.phone}@phone.com`,
      key,
      key_preview: preview,
      key_hash: keyHash,
      key_hash_prefix: keyHash.slice(0, 12),
      group_name: groupName,
      yui_plan_id: row.sub_plan_id || '',
      sub_status: row.sub_status || '',
      sub_source: subSource,
      starts_at_text: startsAt,
      expires_at_text: expiresAt,
      daily_usage_usd: Number(row.daily_usage_usd || 0),
      period_usage_usd: Number(row.period_usage_usd || 0),
      redeemed_at: row.redeemed_at,
      inventory_status: row.inventory_status || '',
      preview_matches: preview === row.api_key_preview,
      hash_matches_inventory: row.inventory_hash ? row.inventory_hash === keyHash : null,
    };
  });
}

function assertLocalPlan(rows) {
  const errors = [];
  if (rows.length !== 15) errors.push(`orders 计划迁移数量应为 15，实际为 ${rows.length}`);

  const keys = new Set();
  const phones = new Set();
  for (const row of rows) {
    if (!row.key || row.key.length < 16) errors.push(`${row.phone_masked} Key 解密结果过短`);
    if (!row.preview_matches) errors.push(`${row.phone_masked} key preview 不匹配`);
    if (row.hash_matches_inventory === false) errors.push(`${row.phone_masked} inventory hash 不匹配`);
    if (keys.has(row.key)) errors.push(`${row.phone_masked} 计划导入 Key 重复`);
    if (phones.has(row.phone)) errors.push(`${row.phone_masked} 订单手机号重复`);
    if (!row.starts_at_text || !row.expires_at_text) errors.push(`${row.phone_masked} 缺少订阅起止时间`);
    keys.add(row.key);
    phones.add(row.phone);
  }

  if (errors.length > 0) {
    throw new Error(`本地 staging 校验失败：\n${errors.join('\n')}`);
  }
}

function preflight(opts, rows, scrub) {
  const payload = JSON.stringify(rows.map((row) => ({
    email: row.email,
    phone_masked: row.phone_masked,
    key: row.key,
    key_preview: row.key_preview,
    key_hash_prefix: row.key_hash_prefix,
    group_name: row.group_name,
    sub_source: row.sub_source,
    yui_plan_id: row.yui_plan_id,
    starts_at_text: row.starts_at_text,
    expires_at_text: row.expires_at_text,
    daily_usage_usd: row.daily_usage_usd,
    period_usage_usd: row.period_usage_usd,
  })));

  const sql = `
WITH input AS (
  SELECT *
  FROM jsonb_to_recordset(${dollarQuote(payload)}::jsonb) AS x(
    email text,
    phone_masked text,
    key text,
    key_preview text,
    key_hash_prefix text,
    group_name text,
    sub_source text,
    yui_plan_id text,
    starts_at_text text,
    expires_at_text text,
    daily_usage_usd numeric,
    period_usage_usd numeric
  )
),
joined AS (
  SELECT
    i.email,
    i.phone_masked,
    i.key_preview,
    i.key_hash_prefix,
    i.group_name,
    i.sub_source,
    i.yui_plan_id,
    i.starts_at_text,
    i.expires_at_text,
    i.daily_usage_usd,
    i.period_usage_usd,
    u.id AS user_id,
    g.id AS group_id,
    ak.id AS existing_key_id,
    ak.user_id AS existing_key_user_id,
    ak.deleted_at AS existing_key_deleted_at,
    us.id AS existing_subscription_id,
    us.status AS existing_subscription_status,
    us.expires_at AS existing_subscription_expires_at
  FROM input i
  LEFT JOIN users u ON u.email = i.email AND u.deleted_at IS NULL
  LEFT JOIN groups g ON g.name = i.group_name AND g.deleted_at IS NULL
  LEFT JOIN api_keys ak ON ak.key = i.key
  LEFT JOIN user_subscriptions us ON us.user_id = u.id AND us.group_id = g.id AND us.deleted_at IS NULL
)
SELECT coalesce(jsonb_agg(to_jsonb(joined) ORDER BY phone_masked), '[]'::jsonb)::text FROM joined;
`;

  return JSON.parse(psql(opts.pgContainer, sql, scrub) || '[]');
}

function summarize(rows, joined) {
  const byPreview = new Map(joined.map((row) => [row.key_preview, row]));
  const details = rows.map((row) => {
    const db = byPreview.get(row.key_preview) || {};
    return {
      phone: row.phone_masked,
      email: row.email.replace(/^(\d{3})\d{4}(\d{4})@/, '$1****$2@'),
      key_preview: row.key_preview,
      key_hash_prefix: row.key_hash_prefix,
      group: row.group_name,
      source: row.sub_source,
      daily_usage_usd: row.daily_usage_usd,
      period_usage_usd: row.period_usage_usd,
      user_found: Boolean(db.user_id),
      group_found: Boolean(db.group_id),
      existing_key: Boolean(db.existing_key_id),
      existing_same_user: Boolean(db.existing_key_id && db.user_id === db.existing_key_user_id),
      existing_subscription: Boolean(db.existing_subscription_id),
    };
  });

  const counts = {
    planned_keys: rows.length,
    active_subscription_source: rows.filter((row) => row.sub_source === 'active_subscription').length,
    manual_order_subscription_source: rows.filter((row) => row.sub_source === 'manual_order_subscription').length,
    users_found: joined.filter((row) => row.user_id).length,
    groups_found: joined.filter((row) => row.group_id).length,
    existing_keys_same_user: joined.filter((row) => row.existing_key_id && row.user_id === row.existing_key_user_id).length,
    existing_key_conflicts: joined.filter((row) => row.existing_key_id && row.user_id !== row.existing_key_user_id).length,
    existing_deleted_key_conflicts: joined.filter((row) => row.existing_key_id && row.existing_key_deleted_at).length,
    existing_subscriptions: joined.filter((row) => row.existing_subscription_id).length,
    daily_usage_usd_total: round6(rows.reduce((sum, row) => sum + row.daily_usage_usd, 0)),
    period_usage_usd_total: round6(rows.reduce((sum, row) => sum + row.period_usage_usd, 0)),
  };

  return { counts, details };
}

function assertPreflight(joined) {
  const missingUsers = joined.filter((row) => !row.user_id);
  const missingGroups = joined.filter((row) => !row.group_id);
  const keyConflicts = joined.filter((row) => row.existing_key_id && row.user_id !== row.existing_key_user_id);
  const deletedKeyConflicts = joined.filter((row) => row.existing_key_id && row.existing_key_deleted_at);

  const errors = [];
  if (missingUsers.length > 0) errors.push(`缺失 Sub2API 用户：${missingUsers.map((row) => row.phone_masked).join(', ')}`);
  if (missingGroups.length > 0) errors.push(`缺失 Sub2API group：${missingGroups.map((row) => row.group_name).join(', ')}`);
  if (keyConflicts.length > 0) errors.push(`Key 已被其他用户占用：${keyConflicts.map((row) => row.key_preview).join(', ')}`);
  if (deletedKeyConflicts.length > 0) errors.push(`Key 与软删除记录冲突：${deletedKeyConflicts.map((row) => row.key_preview).join(', ')}`);

  if (errors.length > 0) {
    throw new Error(`preflight 失败：\n${errors.join('\n')}`);
  }
}

function round6(value) {
  return Math.round(Number(value || 0) * 1_000_000) / 1_000_000;
}

function applyImport(opts, rows, scrub) {
  const payload = JSON.stringify(rows.map((row) => ({
    email: row.email,
    key: row.key,
    key_preview: row.key_preview,
    key_hash_prefix: row.key_hash_prefix,
    group_name: row.group_name,
    sub_source: row.sub_source,
    yui_plan_id: row.yui_plan_id,
    starts_at_text: row.starts_at_text,
    expires_at_text: row.expires_at_text,
    daily_usage_usd: row.daily_usage_usd,
    period_usage_usd: row.period_usage_usd,
  })));

  const sql = `
BEGIN;

CREATE TEMP TABLE tmp_yuiweb_legacy_api_keys AS
SELECT *
FROM jsonb_to_recordset(${dollarQuote(payload)}::jsonb) AS x(
  email text,
  key text,
  key_preview text,
  key_hash_prefix text,
  group_name text,
  sub_source text,
  yui_plan_id text,
  starts_at_text text,
  expires_at_text text,
  daily_usage_usd numeric,
  period_usage_usd numeric
);

WITH mapped AS (
  SELECT
    u.id AS user_id,
    g.id AS group_id,
    t.key,
    t.key_preview,
    t.group_name,
    t.sub_source,
    t.yui_plan_id,
    t.starts_at_text::timestamptz AS starts_at,
    t.expires_at_text::timestamptz AS expires_at,
    t.daily_usage_usd::numeric(20,10) AS daily_usage_usd,
    t.period_usage_usd::numeric(20,10) AS period_usage_usd
  FROM tmp_yuiweb_legacy_api_keys t
  JOIN users u ON u.email = t.email AND u.deleted_at IS NULL
  JOIN groups g ON g.name = t.group_name AND g.deleted_at IS NULL
)
INSERT INTO user_subscriptions (
  user_id,
  group_id,
  starts_at,
  expires_at,
  status,
  daily_window_start,
  weekly_window_start,
  monthly_window_start,
  daily_usage_usd,
  weekly_usage_usd,
  monthly_usage_usd,
  assigned_by,
  assigned_at,
  notes,
  created_at,
  updated_at
)
SELECT
  user_id,
  group_id,
  starts_at,
  expires_at,
  'active',
  date_trunc('day', now()),
  date_trunc('week', now()),
  starts_at,
  daily_usage_usd,
  period_usage_usd,
  period_usage_usd,
  NULL,
  now(),
  CASE
    WHEN sub_source = 'active_subscription'
      THEN 'migrated_from_yuiweb_active_subscription_20260618 plan=' || yui_plan_id
    ELSE 'migrated_from_yuiweb_legacy_key_manual_subscription_20260618 order_expires_at=' || expires_at::text
  END,
  now(),
  now()
FROM mapped
ON CONFLICT (user_id, group_id) WHERE deleted_at IS NULL
DO UPDATE SET
  starts_at = EXCLUDED.starts_at,
  expires_at = EXCLUDED.expires_at,
  status = 'active',
  daily_window_start = EXCLUDED.daily_window_start,
  weekly_window_start = EXCLUDED.weekly_window_start,
  monthly_window_start = EXCLUDED.monthly_window_start,
  daily_usage_usd = EXCLUDED.daily_usage_usd,
  weekly_usage_usd = EXCLUDED.weekly_usage_usd,
  monthly_usage_usd = EXCLUDED.monthly_usage_usd,
  notes = EXCLUDED.notes,
  updated_at = now();

WITH mapped AS (
  SELECT
    u.id AS user_id,
    g.id AS group_id,
    t.key,
    t.key_preview,
    t.starts_at_text::timestamptz AS created_at
  FROM tmp_yuiweb_legacy_api_keys t
  JOIN users u ON u.email = t.email AND u.deleted_at IS NULL
  JOIN groups g ON g.name = t.group_name AND g.deleted_at IS NULL
)
INSERT INTO api_keys (
  user_id,
  key,
  name,
  group_id,
  status,
  ip_whitelist,
  ip_blacklist,
  quota,
  quota_used,
  rate_limit_5h,
  rate_limit_1d,
  rate_limit_7d,
  usage_5h,
  usage_1d,
  usage_7d,
  created_at,
  updated_at
)
SELECT
  user_id,
  key,
  'yui.web legacy key ' || key_preview,
  group_id,
  'active',
  '[]'::jsonb,
  '[]'::jsonb,
  0,
  0,
  0,
  0,
  0,
  0,
  0,
  0,
  created_at,
  now()
FROM mapped
ON CONFLICT (key)
DO UPDATE SET
  name = EXCLUDED.name,
  group_id = EXCLUDED.group_id,
  status = 'active',
  updated_at = now()
WHERE api_keys.user_id = EXCLUDED.user_id
  AND api_keys.deleted_at IS NULL;

COMMIT;

WITH input AS (
  SELECT *
  FROM tmp_yuiweb_legacy_api_keys
),
joined AS (
  SELECT
    i.email,
    i.key_preview,
    i.group_name,
    u.id AS user_id,
    g.id AS group_id,
    ak.id AS api_key_id,
    us.id AS subscription_id
  FROM input i
  JOIN users u ON u.email = i.email AND u.deleted_at IS NULL
  JOIN groups g ON g.name = i.group_name AND g.deleted_at IS NULL
  JOIN api_keys ak ON ak.key = i.key AND ak.deleted_at IS NULL
  JOIN user_subscriptions us ON us.user_id = u.id AND us.group_id = g.id AND us.deleted_at IS NULL
)
SELECT jsonb_build_object(
  'legacy_keys_present', count(*),
  'subscriptions_present', count(subscription_id),
  'rows', coalesce(jsonb_agg(jsonb_build_object(
    'email', regexp_replace(email, '^(\\d{3})\\d{4}(\\d{4})@', '\\1****\\2@'),
    'key_preview', key_preview,
    'group_name', group_name,
    'user_id', user_id,
    'group_id', group_id,
    'api_key_id', api_key_id,
    'subscription_id', subscription_id
  ) ORDER BY email), '[]'::jsonb)
)::text
FROM joined;
`;

  return parseLastJsonLine(psql(opts.pgContainer, sql, scrub));
}

function createBackups(opts) {
  const now = new Date();
  const pad = (value) => String(value).padStart(2, '0');
  const stamp = [
    now.getFullYear(),
    pad(now.getMonth() + 1),
    pad(now.getDate()),
    '-',
    pad(now.getHours()),
    pad(now.getMinutes()),
    pad(now.getSeconds()),
  ].join('');

  const pgTmp = `/tmp/sub2api-before-legacy-api-key-import-${stamp}.dump`;
  const pgLocal = resolve(`.tmp-sub2api-before-legacy-api-key-import-${stamp}.dump`);
  const sqliteLocal = resolve(`.tmp-yuiweb-shop-before-legacy-api-key-import-${stamp}.sqlite`);

  run('pg_dump', 'docker', [
    'exec',
    opts.pgContainer,
    'pg_dump',
    '-U',
    'sub2api',
    '-d',
    'sub2api',
    '--format=custom',
    `--file=${pgTmp}`,
  ], { allowStdout: true });

  run('docker cp pg backup', 'docker', [
    'cp',
    `${opts.pgContainer}:${pgTmp}`,
    pgLocal,
  ], { allowStdout: true });

  copyFileSync(opts.yuiDb, sqliteLocal);

  return {
    postgres_dump: pgLocal,
    yuiweb_sqlite: sqliteLocal,
  };
}

function clearRedisCaches(opts, applyResult, rows, scrub) {
  const commandLines = [];
  const rowByPreview = new Map(rows.map((row) => [row.key_preview, row]));
  for (const row of applyResult.rows || []) {
    const source = rowByPreview.get(row.key_preview);
    if (!source) continue;
    commandLines.push(`DEL apikey:auth:${source.key}`);
    commandLines.push(`DEL billing:sub:${row.user_id}:${row.group_id}`);
    commandLines.push(`DEL apikey:rate:${row.api_key_id}`);
  }
  if (commandLines.length === 0) return { commands: 0, deleted_total: 0 };

  const output = redisCli(opts.redisContainer, `${commandLines.join('\n')}\n`, scrub);
  const deletedTotal = output.split(/\s+/).filter(Boolean).reduce((sum, value) => {
    const parsed = Number(value);
    return Number.isFinite(parsed) ? sum + parsed : sum;
  }, 0);

  return {
    commands: commandLines.length,
    deleted_total: deletedTotal,
  };
}

async function main() {
  const opts = parseArgs(process.argv.slice(2));
  ensureInputFiles(opts);

  const env = readEnv(opts.yuiEnv);
  const secret = env.SHOP_API_KEY_ENCRYPTION_SECRET;
  if (!secret) throw new Error('yui.web .env 缺少 SHOP_API_KEY_ENCRYPTION_SECRET');

  const rows = loadOrders(opts, secret);
  const scrub = scrubber(rows.map((row) => row.key));
  assertLocalPlan(rows);

  const joined = preflight(opts, rows, scrub);
  const summary = summarize(rows, joined);
  assertPreflight(joined);

  if (!opts.apply) {
    console.log(JSON.stringify({
      mode: 'dry-run',
      ok_to_apply: true,
      ...summary,
    }, null, 2));
    return;
  }

  const backups = opts.backup ? createBackups(opts) : null;
  const applyResult = applyImport(opts, rows, scrub);
  const redis = clearRedisCaches(opts, applyResult, rows, scrub);

  console.log(JSON.stringify({
    mode: 'apply',
    backups,
    preflight: summary.counts,
    apply: {
      legacy_keys_present: applyResult.legacy_keys_present,
      subscriptions_present: applyResult.subscriptions_present,
      rows: applyResult.rows,
    },
    redis,
  }, null, 2));
}

main().catch((error) => {
  console.error(error.message);
  process.exit(1);
});
