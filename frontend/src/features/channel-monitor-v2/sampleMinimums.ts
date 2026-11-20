export const sampleRanges = ['90m', '24h', '7d', '30d'] as const
export type SampleRange = (typeof sampleRanges)[number]

// Empty number inputs mean "use the common minimum", not zero requests.
export function normalizeSampleMinimums(
  value?: Partial<Record<SampleRange, number>>,
): Partial<Record<SampleRange, number>> {
  const result: Partial<Record<SampleRange, number>> = {}
  for (const window of sampleRanges) {
    const minimum = value?.[window]
    if (typeof minimum === 'number' && Number.isFinite(minimum) && minimum >= 1) {
      result[window] = Math.min(10000, Math.trunc(minimum))
    }
  }
  return result
}
