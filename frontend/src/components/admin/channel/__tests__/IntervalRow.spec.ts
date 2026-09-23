import { enableAutoUnmount, mount } from '@vue/test-utils'
import { afterEach, describe, expect, it, vi } from 'vitest'
import IntervalRow from '../IntervalRow.vue'
import type { IntervalFormEntry } from '../types'

vi.mock('vue-i18n', () => ({ useI18n: () => ({ t: (key: string) => key }) }))
enableAutoUnmount(afterEach)
const interval: IntervalFormEntry = {
  min_tokens: 0, max_tokens: null, tier_label: '', sort_order: 0,
  input_price: null, output_price: null, cache_write_price: null, cache_read_price: null,
  input_multiplier: null, output_multiplier: null, cache_write_multiplier: null,
  cache_read_multiplier: null, per_request_price: null,
}

describe('pricing interval token bounds', () => {
  it.each([
    ['1e6', 1000000, 1000000],
    ['2.72e5', 272000, 272000],
    ['128000', 128000, 128000],
    ['', 0, null],
  ])('parses %s without losing the exponent', async (value, min, max) => {
    const wrapper = mount(IntervalRow, { props: { interval, mode: 'token' } })
    const inputs = wrapper.findAll('input[type="number"]')
    await inputs[0].setValue(value)
    await inputs[1].setValue(value)
    expect(wrapper.emitted('update')?.[0]?.[0]).toMatchObject({ min_tokens: min })
    expect(wrapper.emitted('update')?.[1]?.[0]).toMatchObject({ max_tokens: max })
  })
})
