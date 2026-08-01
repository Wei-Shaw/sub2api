type TranslateFn = (key: string, ...args: unknown[]) => string

type TextPattern = {
  needles: string[]
  key: string
}

const zh = (hex: string): string =>
  hex
    .split(/\s+/)
    .filter(Boolean)
    .map((part) => String.fromCharCode(Number.parseInt(part, 16)))
    .join('')

const pattern = (key: string, ...needles: string[]): TextPattern => ({
  key,
  needles: needles.filter(Boolean)
})

const textPatterns: TextPattern[] = [
  pattern('apiKeyGroupDeleted', `api key ${zh('6240 5c5e 5206 7ec4 5df2 5220 9664')}`, 'API key group was deleted'),
  pattern('apiKeyGroupDisabled', `api key ${zh('6240 5c5e 5206 7ec4 5df2 505c 7528')}`, 'API key group is disabled'),
  pattern('apiKeyFiveHourQuotaExhausted', `api key 5${zh('5c0f 65f6 9650 989d 5df2 7528 5b8c')}`, 'API key 5-hour quota exhausted'),
  pattern('apiKeyDailyQuotaExhausted', `api key ${zh('65e5 9650 989d 5df2 7528 5b8c')}`, 'API key daily quota exhausted'),
  pattern('apiKeyWeeklyQuotaExhausted', `api key 7${zh('5929 9650 989d 5df2 7528 5b8c')}`, 'API key 7-day quota exhausted'),
  pattern('apiKeyQuotaExhausted', `api key ${zh('989d 5ea6 5df2 7528 5b8c')}`, 'API key quota exhausted'),
  pattern('apiKeyExpired', `api key ${zh('5df2 8fc7 671f')}`, 'API key expired'),

  pattern('dailyReport', zh('65e5 62a5'), 'Daily report'),
  pattern('weeklyReport', zh('5468 62a5'), 'Weekly report'),
  pattern('errorDigest', zh('9519 8bef 6458 8981'), 'Error digest'),
  pattern('accountHealth', zh('8d26 53f7 5065 5eb7'), 'Account health'),

  pattern('dashboardCacheReadFailed', zh('4eea 8868 76d8 7f13 5b58 8bfb 53d6 5931 8d25'), 'dashboard cache read failed'),
  pattern('dashboardCacheRefreshFailed', zh('4eea 8868 76d8 7f13 5b58 5f02 6b65 5237 65b0 5931 8d25'), 'dashboard cache async refresh failed'),
  pattern('dashboardCacheSerializeFailed', zh('4eea 8868 76d8 7f13 5b58 5e8f 5217 5316 5931 8d25'), 'dashboard cache serialization failed'),
  pattern('dashboardCacheWriteFailed', zh('4eea 8868 76d8 7f13 5b58 5199 5165 5931 8d25'), 'dashboard cache write failed'),
  pattern('dashboardCacheClearFailed', zh('4eea 8868 76d8 7f13 5b58 6e05 7406 5931 8d25'), 'dashboard cache cleanup failed'),
  pattern('dashboardCacheInvalidCleared', zh('4eea 8868 76d8 7f13 5b58 5f02 5e38 002c 5df2 6e05 7406'), 'dashboard cache invalid; cleared'),
  pattern('aggregationWatermarkReadFailed', zh('8bfb 53d6 805a 5408 6c34 4f4d 5931 8d25'), 'failed to read aggregation watermark', 'failed to read watermark'),

  pattern('aggregationJobDisabled', zh('805a 5408 4f5c 4e1a 5df2 7981 7528'), 'aggregation job disabled'),
  pattern('aggregationJobStarted', zh('805a 5408 4f5c 4e1a 542f 52a8'), 'aggregation job started'),
  pattern('backfillDisabled', zh('56de 586b 5df2 7981 7528'), 'backfill disabled'),
  pattern('backfillRejected', zh('56de 586b 88ab 62d2 7edd'), 'backfill rejected'),
  pattern('backfillFailed', zh('56de 586b 5931 8d25'), 'backfill failed'),
  pattern('recomputeFailed', zh('91cd 65b0 8ba1 7b97 5931 8d25'), 'recompute failed'),
  pattern('recomputeAbandoned', zh('91cd 65b0 8ba1 7b97 653e 5f03'), 'recompute abandoned'),
  pattern('recomputeStartFailed', zh('542f 52a8 91cd 7b97 5931 8d25'), 'failed to start recompute'),
  pattern('recomputeCompleted', zh('91cd 65b0 8ba1 7b97 5b8c 6210'), 'recompute completed'),
  pattern('aggregationFailed', zh('805a 5408 5931 8d25'), 'aggregation failed'),
  pattern('watermarkUpdateFailed', zh('66f4 65b0 6c34 4f4d 5931 8d25'), 'failed to update watermark'),
  pattern('aggregationCompleted', zh('805a 5408 5b8c 6210'), 'aggregation completed'),
  pattern('backfillAggregationCompleted', zh('56de 586b 805a 5408 5b8c 6210'), 'backfill aggregation completed'),
  pattern('partitionCheckFailed', zh('5206 533a 68c0 67e5 5931 8d25'), 'partition check failed'),
  pattern('aggregationRetentionCleanupFailed', zh('805a 5408 4fdd 7559 6e05 7406 5931 8d25'), 'aggregation retention cleanup failed'),
  pattern('usageLogRetentionCleanupFailed', `usage_logs ${zh('4fdd 7559 6e05 7406 5931 8d25')}`, 'usage_logs retention cleanup failed'),
  pattern('usageDedupRetentionCleanupFailed', `usage_billing_dedup ${zh('4fdd 7559 6e05 7406 5931 8d25')}`, 'usage_billing_dedup retention cleanup failed'),

  pattern('backupConfigLoadFailed', zh('52a0 8f7d 5b9a 65f6 5907 4efd 914d 7f6e 5931 8d25'), 'failed to load scheduled backup config'),
  pattern('backupConfigApplyFailed', zh('5e94 7528 5b9a 65f6 5907 4efd 914d 7f6e 5931 8d25'), 'failed to apply scheduled backup config'),
  pattern('backupEnabled', zh('5b9a 65f6 5907 4efd 5df2 542f 7528'), 'scheduled backup enabled'),
  pattern('backupDisabled', zh('5b9a 65f6 5907 4efd 5df2 505c 7528'), 'scheduled backup disabled'),
  pattern('backupStarted', zh('5f00 59cb 6267 884c 5b9a 65f6 5907 4efd'), 'started scheduled backup'),
  pattern('backupSkipped', zh('5b9a 65f6 5907 4efd 8df3 8fc7'), 'scheduled backup skipped'),
  pattern('backupFailed', zh('5b9a 65f6 5907 4efd 5931 8d25'), 'scheduled backup failed'),
  pattern('backupCompleted', zh('5b9a 65f6 5907 4efd 5b8c 6210'), 'scheduled backup completed'),
  pattern('backupCleanupFailed', zh('6e05 7406 8fc7 671f 5907 4efd 5931 8d25'), 'failed to clean expired backups'),
  pattern('backupRecordSaveFailed', zh('4fdd 5b58 5907 4efd 8bb0 5f55 5931 8d25'), 'failed to save backup record'),
  pattern('restoreRecordSaveFailed', zh('4fdd 5b58 6062 590d 8bb0 5f55 5931 8d25'), 'failed to save restore record'),
  pattern('backupAutoCleaned', zh('81ea 52a8 6e05 7406 4e86'), 'automatically cleaned'),
  pattern('s3SecretDecryptFailed', `S3 SecretAccessKey ${zh('89e3 5bc6 5931 8d25')}`, 'S3 SecretAccessKey decrypt failed'),

  pattern('openaiCodexAllowed', `OpenAI codex_cli_only ${zh('653e 884c 8bf7 6c42')}`, 'OpenAI codex_cli_only allowed official-client request'),
  pattern('openaiCodexRejected', `OpenAI codex_cli_only ${zh('62d2 7edd 975e 5b98 65b9 5ba2 6237 7aef 8bf7 6c42')}`, 'OpenAI codex_cli_only rejected non-official-client request'),
  pattern('openaiInstructionsRequired', `OpenAI ${zh('4e0a 6e38 8fd4 56de')} Instructions are required${zh('ff0c 5df2 8bb0 5f55 8bf7 6c42 8be6 60c5 7528 4e8e 6392 67e5')}`, 'OpenAI upstream returned Instructions are required; request details were recorded for troubleshooting', 'OpenAI upstream returned Instructions are required; request details recorded for troubleshooting'),
  pattern('openaiInstructionsMissing', `OpenAI passthrough ${zh('672c 5730 62e6 622a ff1a')}Codex ${zh('8bf7 6c42 7f3a 5c11 6709 6548')} instructions`, 'OpenAI passthrough blocked locally: Codex request is missing valid instructions'),

  pattern('loadCodeAssistTemporaryFailed', `LoadCodeAssist ${zh('4e34 65f6 5931 8d25 ff0c 4fdd 7559 65e7')} project_id`, 'LoadCodeAssist temporary failure; keeping previous project_id'),
  pattern('loadCodeAssistMissingProject', `LoadCodeAssist ${zh('5931 8d25 ff0c')}project_id ${zh('7f3a 5931 ff0c 4f46')} token ${zh('5df2 66f4 65b0 ff0c 5c06 5728 4e0b 6b21 5237 65b0 65f6 91cd 8bd5')}`, 'LoadCodeAssist failed; project_id missing, token updated, will retry on next refresh', 'LoadCodeAssist failed; project_id missing but token was updated; will retry on next refresh'),

  pattern('logFileOutputFallback', zh('65e5 5fd7 6587 4ef6 8f93 51fa 521d 59cb 5316 5931 8d25 002c 964d 7ea7 4e3a 4ec5 6807 51c6 8f93 51fa'), 'log file output initialization failed; falling back to standard output only', 'log file output initialization failed; falling back to stdout-only'),
  pattern('cspNonceFallback', zh('964d 7ea7 4e3a 65e0 0020 006e 006f 006e 0063 0065 0020 7684 0020 0043 0053 0050'), 'fallback to CSP without nonce'),

  pattern('quotaCacheInvalidationFailed', zh('6c42 6d41 91cf 7f13 5b58 5931 6548'), 'quota cache invalidation failed'),
  pattern('quotaLimitMayBeDelayed', zh('751f 6548 53ef 80fd 5ef6 8fdf 81f3 0020 0073 0065 006e 0074 0069 006e 0065 006c 0020 0054 0054 004c'), 'limit may be delayed until sentinel TTL'),
  pattern('quotaWindowResetMayBeDelayed', zh('7a97 53e3 91cd 7f6e 53ef 80fd 5ef6 8fdf'), 'window reset may be delayed')
]

const escapeRegExp = (value: string): string => value.replace(/[.*+?^${}()|[\]\\]/g, '\\$&')

export const opsMessageTranslationKeys = [...new Set(textPatterns.map((item) => item.key))]

export function translateOpsText(raw: unknown, t: TranslateFn): string {
  let output = String(raw ?? '').trim()
  if (!output) return ''

  for (const item of textPatterns) {
    const replacement = t(`admin.ops.messageTranslations.${item.key}`)
    for (const needle of item.needles) {
      output = output.replace(new RegExp(escapeRegExp(needle), 'gi'), replacement)
    }
  }

  return output
}

export function formatOpsLogMessage(raw: unknown, t: TranslateFn): string {
  const text = String(raw ?? '').trim()
  if (!text) return ''

  if (text.startsWith('{') || text.startsWith('[')) {
    try {
      const parsed = JSON.parse(text)
      const message = extractMessageField(parsed)
      if (message) return translateOpsText(message, t)
      return translateOpsText(JSON.stringify(translateJsonStrings(parsed, t)).slice(0, 150), t)
    } catch {
      // Fall through to plain-text translation.
    }
  }

  return translateOpsText(text.length > 200 ? `${text.slice(0, 200)}...` : text, t)
}

export function prettyOpsJSON(raw: unknown, t: TranslateFn): string {
  const text = String(raw ?? '').trim()
  if (!text) return 'N/A'

  try {
    const parsed = JSON.parse(text)
    return JSON.stringify(translateJsonStrings(parsed, t), null, 2)
  } catch {
    return translateOpsText(text, t)
  }
}

function extractMessageField(value: unknown): string {
  if (!value || typeof value !== 'object') return ''
  const record = value as Record<string, unknown>
  const error = record.error
  if (error && typeof error === 'object') {
    const message = (error as Record<string, unknown>).message
    if (typeof message === 'string' && message.trim()) return message.trim()
  }
  for (const key of ['message', 'detail', 'error_message']) {
    const message = record[key]
    if (typeof message === 'string' && message.trim()) return message.trim()
  }
  return ''
}

function translateJsonStrings(value: unknown, t: TranslateFn): unknown {
  if (typeof value === 'string') return translateOpsText(value, t)
  if (Array.isArray(value)) return value.map((item) => translateJsonStrings(item, t))
  if (!value || typeof value !== 'object') return value

  const out: Record<string, unknown> = {}
  for (const [key, item] of Object.entries(value)) {
    out[key] = translateJsonStrings(item, t)
  }
  return out
}
