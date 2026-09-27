import { bench, describe } from 'vitest'
import { formatCurrency, formatDate, formatNumber } from '../format'

// Run: pnpm exec vitest bench src/utils/__tests__/format.bench.ts
const CALLS = 10_000
const date = new Date('2026-01-02T03:04:05Z')

describe(`utils/format x${CALLS} calls`, () => {
  bench('formatDate', () => {
    for (let i = 0; i < CALLS; i++) formatDate(date)
  })

  bench('formatNumber (compact)', () => {
    for (let i = 0; i < CALLS; i++) formatNumber(123456 + i)
  })

  bench('formatCurrency', () => {
    for (let i = 0; i < CALLS; i++) formatCurrency(i / 100)
  })
})
