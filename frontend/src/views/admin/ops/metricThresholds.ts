import type { OpsMetricThresholds } from './types'

export const DEFAULT_OPS_METRIC_THRESHOLDS: Required<OpsMetricThresholds> = {
  sla_percent_min: 99.5,
  ttft_p99_ms_max: 500,
  request_error_rate_percent_max: 5,
  upstream_error_rate_percent_max: 5,
  health_score_error_rate_full_percent: 1,
  health_score_error_rate_zero_percent: 10,
  health_score_ttft_p99_full_ms: 1000,
  health_score_ttft_p99_zero_ms: 3000
}

export function normalizeOpsMetricThresholds(thresholds?: OpsMetricThresholds | null): Required<OpsMetricThresholds> {
  const cfg = thresholds ?? {}
  return {
    sla_percent_min: cfg.sla_percent_min ?? DEFAULT_OPS_METRIC_THRESHOLDS.sla_percent_min,
    ttft_p99_ms_max: cfg.ttft_p99_ms_max ?? DEFAULT_OPS_METRIC_THRESHOLDS.ttft_p99_ms_max,
    request_error_rate_percent_max: cfg.request_error_rate_percent_max ?? DEFAULT_OPS_METRIC_THRESHOLDS.request_error_rate_percent_max,
    upstream_error_rate_percent_max: cfg.upstream_error_rate_percent_max ?? DEFAULT_OPS_METRIC_THRESHOLDS.upstream_error_rate_percent_max,
    health_score_error_rate_full_percent: cfg.health_score_error_rate_full_percent ?? DEFAULT_OPS_METRIC_THRESHOLDS.health_score_error_rate_full_percent,
    health_score_error_rate_zero_percent: cfg.health_score_error_rate_zero_percent ?? DEFAULT_OPS_METRIC_THRESHOLDS.health_score_error_rate_zero_percent,
    health_score_ttft_p99_full_ms: cfg.health_score_ttft_p99_full_ms ?? DEFAULT_OPS_METRIC_THRESHOLDS.health_score_ttft_p99_full_ms,
    health_score_ttft_p99_zero_ms: cfg.health_score_ttft_p99_zero_ms ?? DEFAULT_OPS_METRIC_THRESHOLDS.health_score_ttft_p99_zero_ms
  }
}

export function collectOpsMetricThresholdErrors(
  thresholds: OpsMetricThresholds | null | undefined,
  t: (key: string) => string
): string[] {
  const errors: string[] = []
  if (!thresholds) return errors

  const percentFields: Array<[keyof OpsMetricThresholds, string]> = [
    ['sla_percent_min', 'slaMinPercentRange'],
    ['request_error_rate_percent_max', 'requestErrorRateMaxRange'],
    ['upstream_error_rate_percent_max', 'upstreamErrorRateMaxRange'],
    ['health_score_error_rate_full_percent', 'healthScoreErrorRateRange'],
    ['health_score_error_rate_zero_percent', 'healthScoreErrorRateRange']
  ]
  for (const [field, messageKey] of percentFields) {
    const value = thresholds[field]
    if (value != null && (!Number.isFinite(value) || value < 0 || value > 100)) {
      errors.push(t(`validation.${messageKey}`))
    }
  }

  const nonNegativeFields: Array<[keyof OpsMetricThresholds, string]> = [
    ['ttft_p99_ms_max', 'ttftP99MaxRange'],
    ['health_score_ttft_p99_full_ms', 'healthScoreTTFTRange'],
    ['health_score_ttft_p99_zero_ms', 'healthScoreTTFTRange']
  ]
  for (const [field, messageKey] of nonNegativeFields) {
    const value = thresholds[field]
    if (value != null && (!Number.isFinite(value) || value < 0)) {
      errors.push(t(`validation.${messageKey}`))
    }
  }

  const errFull = thresholds.health_score_error_rate_full_percent
  const errZero = thresholds.health_score_error_rate_zero_percent
  if (errFull != null && errZero != null && Number.isFinite(errFull) && Number.isFinite(errZero) && errFull >= errZero) {
    errors.push(t('validation.healthScoreErrorRateOrder'))
  }

  const ttftFull = thresholds.health_score_ttft_p99_full_ms
  const ttftZero = thresholds.health_score_ttft_p99_zero_ms
  if (ttftFull != null && ttftZero != null && Number.isFinite(ttftFull) && Number.isFinite(ttftZero) && ttftFull >= ttftZero) {
    errors.push(t('validation.healthScoreTTFTOrder'))
  }

  return errors
}
