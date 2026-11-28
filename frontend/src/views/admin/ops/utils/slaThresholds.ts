export type ThresholdLevel = 'normal' | 'warning' | 'critical'

export function getSLAThresholdLevel(
  slaPercent: number | null,
  threshold: number | null | undefined
): ThresholdLevel {
  if (slaPercent == null || threshold == null) return 'normal'

  // SLA is higher-is-better: below the configured boundary is critical, and a
  // 0.1 percentage-point buffer above it remains warning.
  const warningBuffer = 0.1
  if (slaPercent < threshold) return 'critical'
  if (slaPercent < threshold + warningBuffer) return 'warning'
  return 'normal'
}

export function getSLAProgressPercent(
  slaPercent: number | null,
  threshold: number | null | undefined
): number {
  if (slaPercent == null || threshold == null) return 0

  // Map the range from the configured boundary to 100% onto the progress bar.
  // This keeps the bar and threshold color driven by the same setting.
  const progressSpan = 100 - threshold
  if (progressSpan <= 0) return slaPercent >= 100 ? 100 : 0
  return Math.min(100, Math.max(((slaPercent - threshold) / progressSpan) * 100, 0))
}
