/** Preserve nonzero usage and a small remaining balance at the display edges. */
export function formatQuotaPercent(value: number): string {
  if (!Number.isFinite(value)) return '—'
  if (value <= 0) return '0%'
  if (value >= 100) return '100%'
  if (value < 0.1) return '<0.1%'
  if (value > 99.9) return '>99.9%'
  return `${Number(value.toFixed(1))}%`
}
