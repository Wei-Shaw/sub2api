import { shallowMount REDACTED from '@vue/test-utils'
import { describe, expect, it, vi REDACTED from 'vitest'
import PricingEntryCard from '../PricingEntryCard.vue'
import type { PricingFormEntry REDACTED from '../types'

vi.mock('vue-i18n', async importOriginal => ({
  ...await importOriginal<typeof import('vue-i18n')>(),
  useI18n: () => ({ t: (key: string) => key REDACTED),
REDACTED))

function createEntry(billingMode: PricingFormEntry['billing_mode'] = 'token'): PricingFormEntry {
  return {
    models: [],
    billing_mode: billingMode,
    input_price: null,
    output_price: null,
    cache_write_price: null,
    cache_read_price: null,
    image_input_price: null,
    image_output_price: null,
    per_request_price: null,
    intervals: [],
    time_pricing: {
      timezone: 'Asia/Shanghai',
      periods: [{ start_time: '09:00', end_time: '12:00', multiplier: '2.00' REDACTED],
    REDACTED,
  REDACTED
REDACTED

describe('PricingEntryCard time pricing visibility', () => {
  it('is hidden by default', () => {
    const wrapper = shallowMount(PricingEntryCard, {
      props: { entry: createEntry() REDACTED,
    REDACTED)

    expect(wrapper.findComponent({ name: 'TimePricingSection' REDACTED).exists()).toBe(false)
  REDACTED)

  it('is shown for token pricing when explicitly enabled', () => {
    const wrapper = shallowMount(PricingEntryCard, {
      props: { entry: createEntry(), enableTimePricing: true REDACTED,
    REDACTED)

    expect(wrapper.findComponent({ name: 'TimePricingSection' REDACTED).exists()).toBe(true)
  REDACTED)

  it('is hidden for non-token pricing even when explicitly enabled', () => {
    const wrapper = shallowMount(PricingEntryCard, {
      props: { entry: createEntry('per_request'), enableTimePricing: true REDACTED,
    REDACTED)

    expect(wrapper.findComponent({ name: 'TimePricingSection' REDACTED).exists()).toBe(false)
  REDACTED)

  it('clears time periods when changing billing mode', () => {
    const entry = createEntry()
    const wrapper = shallowMount(PricingEntryCard, {
      props: { entry, enableTimePricing: true REDACTED,
    REDACTED)

    wrapper.findComponent({ name: 'Select' REDACTED).vm.$emit('update:modelValue', 'image')

    expect(wrapper.emitted('update')?.[0]?.[0]).toEqual({
      ...entry,
      billing_mode: 'image',
      intervals: [],
      time_pricing: { timezone: 'Asia/Shanghai', periods: [] REDACTED,
    REDACTED)
    expect(entry.time_pricing.periods).toHaveLength(1)
  REDACTED)
REDACTED)
