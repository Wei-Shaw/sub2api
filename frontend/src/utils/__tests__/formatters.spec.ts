import { describe, expect, it } from 'vitest'

import { formatTokenCacheRate, formatTokensCompact } from '../formatters'

describe('usage token formatters', () => {
  it('formats compact token counts', () => {
    expect(formatTokensCompact(999)).toBe('999')
    expect(formatTokensCompact(1200)).toBe('1.20K')
    expect(formatTokensCompact(1_250_000)).toBe('1.25M')
  })

  it('formats token cache rate as a two-decimal percentage', () => {
    expect(formatTokenCacheRate(4_010, 95_990)).toBe('95.99%')
    expect(formatTokenCacheRate(0, 0)).toBe('0.00%')
  })
})
