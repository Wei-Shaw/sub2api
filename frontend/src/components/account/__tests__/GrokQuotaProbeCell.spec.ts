import { flushPromises, mount REDACTED from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi REDACTED from 'vitest'
import GrokQuotaProbeCell from '../GrokQuotaProbeCell.vue'
import type { Account REDACTED from '@/types'

const { queryQuota REDACTED = vi.hoisted(() => ({
  queryQuota: vi.fn()
REDACTED))

vi.mock('@/api/admin', () => ({
  adminAPI: {
    grok: { queryQuota REDACTED
  REDACTED
REDACTED))

vi.mock('vue-i18n', () => ({
  useI18n: () => ({
    t: (key: string, params?: Record<string, unknown>) =>
      params?.percent == null ? key : `${keyREDACTED:${params.percentREDACTED`
  REDACTED)
REDACTED))

const account = {
  id: 99,
  platform: 'grok',
  type: 'oauth'
REDACTED as Account

describe('GrokQuotaProbeCell', () => {
  beforeEach(() => {
    queryQuota.mockReset()
  REDACTED)

  it('keeps billing data while exposing a failed Free quota fallback', async () => {
    queryQuota.mockResolvedValue({
      source: 'hybrid_probe',
      billing: { period_type: 'weekly', usage_percent: null REDACTED,
      headers_observed: false,
      reset_supported: false,
      fetched_at: 1,
      probe_error: 'upstream returned 402 for probe model "grok-4.5"'
    REDACTED)
    const wrapper = mount(GrokQuotaProbeCell, { props: { account REDACTED REDACTED)

    await wrapper.get('button').trigger('click')
    await flushPromises()

    expect(wrapper.text()).toContain('upstream returned 402 for probe model "grok-4.5"')
    expect(wrapper.emitted('probed')?.[0]?.[0]).toMatchObject({
      billing: { period_type: 'weekly', usage_percent: null REDACTED,
      probe_error: 'upstream returned 402 for probe model "grok-4.5"'
    REDACTED)
  REDACTED)
REDACTED)
