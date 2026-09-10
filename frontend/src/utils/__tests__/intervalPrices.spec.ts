import { describe, expect, it } from 'vitest'
import { resolveIntervalPrices } from '../pricing'

const base = { input_price: 10, output_price: 50, cache_write_price: 12.5, cache_write_1h_price: 30, cache_read_price: 2 }
const interval = { min_tokens: 272000, max_tokens: null, input_price: null, output_price: null, cache_write_price: null, cache_write_1h_price: null, cache_read_price: null, per_request_price: null }

describe('resolveIntervalPrices', () => {
  it('inherits base prices without overrides', () => {
    expect(resolveIntervalPrices(interval, base)).toMatchObject(base)
  })
  it('applies cache multiplier to both durations', () => {
    expect(resolveIntervalPrices({ ...interval, cache_write_multiplier: 2 }, base)).toMatchObject({ cache_write_price: 25, cache_write_1h_price: 60 })
  })
  it('uses explicit cache write for both durations before multiplier', () => {
    expect(resolveIntervalPrices({ ...interval, cache_write_price: 20, cache_write_multiplier: 2 }, base)).toMatchObject({ cache_write_price: 20, cache_write_1h_price: 20 })
  })
  it('preserves explicit zero and separate 1h override', () => {
    expect(resolveIntervalPrices({ ...interval, cache_write_price: 0, cache_write_1h_price: 7, cache_write_multiplier: 2 }, base)).toMatchObject({ cache_write_price: 0, cache_write_1h_price: 7 })
  })
})
