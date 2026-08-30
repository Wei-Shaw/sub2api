import { defineComponent } from 'vue'
import { flushPromises, mount } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'

const api = vi.hoisted(() => ({
  getConfig: vi.fn(),
  updateConfig: vi.fn(),
  getRuntime: vi.fn(),
  listEvidence: vi.fn(),
  listRuns: vi.fn(),
  listModels: vi.fn(),
  updateModelStatus: vi.fn(),
  discoverModels: vi.fn(),
  createRun: vi.fn(),
  applyRun: vi.fn(),
  rollbackRun: vi.fn(),
}))
const accountList = vi.hoisted(() => vi.fn())
const channelList = vi.hoisted(() => vi.fn())
const showSuccess = vi.hoisted(() => vi.fn())
const showError = vi.hoisted(() => vi.fn())

vi.mock('@/api/admin/upstreamPriceMonitor', () => ({ default: api }))
vi.mock('@/api/admin/accounts', () => ({ default: { list: accountList } }))
vi.mock('@/api/admin/channels', () => ({ default: { list: channelList } }))
vi.mock('@/stores/app', () => ({ useAppStore: () => ({ showSuccess, showError }) }))
vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return {
    ...actual,
    useI18n: () => ({
      t: (key: string, params?: Record<string, unknown>) => {
        if (!params) return key
        return `${key}:${JSON.stringify(params)}`
      },
    }),
  }
})

import UpstreamPriceMonitorView from '../UpstreamPriceMonitorView.vue'

const config = {
  enabled: true,
  mode: 'observe' as const,
  interval_minutes: 15,
  markup: 1.2,
  display_multiplier_decimals: 3,
  account_ids: [7],
  channel_ids: [3],
  domestic_models: ['deepseek-v4-flash-0731'],
  passive_sample_max_age_minutes: 60,
  active_probe_enabled: true,
}

const completedRun = {
  id: 1,
  trigger: 'manual',
  status: 'completed' as const,
  mode: 'observe',
  dry_run: true,
  started_at: '2026-08-30T00:00:00Z',
  matched_models: 1,
  mismatched_models: 0,
  probe_cost: 0.001,
  snapshot_hash: 'abcdef1234567890',
}

const AppLayoutStub = defineComponent({ template: '<main><slot /></main>' })
const ToggleStub = defineComponent({
  props: { modelValue: Boolean },
  emits: ['update:modelValue'],
  template: '<button type="button" role="switch" @click="$emit(\'update:modelValue\', !modelValue)" />',
})
const ConfirmDialogStub = defineComponent({
  props: { show: Boolean, title: String, message: String },
  emits: ['confirm', 'cancel'],
  template: '<div v-if="show" data-testid="confirm-dialog"><button data-testid="confirm-action" @click="$emit(\'confirm\')">confirm</button></div>',
})

function mountView() {
  return mount(UpstreamPriceMonitorView, {
    global: {
      stubs: {
        AppLayout: AppLayoutStub,
        Icon: true,
        Toggle: ToggleStub,
        Pagination: true,
        ConfirmDialog: ConfirmDialogStub,
      },
    },
  })
}

describe('UpstreamPriceMonitorView', () => {
  beforeEach(() => {
    Object.values(api).forEach(mock => mock.mockReset())
    accountList.mockReset()
    channelList.mockReset()
    showSuccess.mockReset()
    showError.mockReset()

    api.getConfig.mockResolvedValue(config)
    api.updateConfig.mockImplementation(async value => value)
    api.getRuntime.mockResolvedValue({
      status: 'idle',
      last_run_at: null,
      next_run_at: null,
      consecutive_failures: 0,
      last_error: '',
      today_probe_cost: 0,
      coverage: { trusted: 1, total: 1 },
      key_exclusive: true,
    })
    api.listEvidence.mockResolvedValue({
      items: [{
        model: 'deepseek-v4-flash-0731',
        account_id: 7,
        billing_mode: 'token',
        status: 'trusted',
        source: 'user_request',
        reconciliation_status: 'closed',
        observed_at: '2026-08-30T00:00:00Z',
        sample_count: 3,
        prices: { input_per_million: 0.16, output_per_million: 0.47, cache_read_per_million: 0.016 },
        current_prices: { input_per_million: 0.18, output_per_million: 0.5 },
        suggested_prices: { input_per_million: 0.192, output_per_million: 0.564 },
        display_multiplier_current: 0.1,
        display_multiplier_suggested: 0.12,
      }],
    })
    api.listRuns.mockResolvedValue({ items: [completedRun], total: 1 })
    api.listModels.mockResolvedValue({ items: [{
      model: 'deepseek-new-v1', status: 'discovered', domestic_candidate: true,
      seen_account_count: 1, expected_account_count: 1, missing_runs: 0, discovery_complete: true, updated_at: '2026-08-30T00:00:00Z',
    }] })
    api.updateModelStatus.mockResolvedValue({
      model: 'deepseek-new-v1', status: 'managed', domestic_candidate: true,
      seen_account_count: 1, expected_account_count: 1, missing_runs: 0, discovery_complete: true, updated_at: '2026-08-30T00:00:00Z',
    })
    api.discoverModels.mockResolvedValue({ items: [{
      model: 'deepseek-new-v1', status: 'discovered', domestic_candidate: true,
      seen_account_count: 1, expected_account_count: 1, missing_runs: 0, discovery_complete: true, updated_at: '2026-08-30T00:00:00Z',
    }] })
    api.createRun.mockResolvedValue({ ...completedRun, id: 2, status: 'running' })
    api.applyRun.mockResolvedValue({ ...completedRun, applied_at: '2026-08-30T00:01:00Z', rollback_available: true })
    api.rollbackRun.mockResolvedValue({ ...completedRun, applied_at: null, rollback_available: false })
    accountList.mockResolvedValue({ items: [{ id: 7, name: 'x5m5x-payg', platform: 'openai' }], total: 1 })
    channelList.mockResolvedValue({ items: [{ id: 3, name: 'x5m5x token' }], total: 1 })
  })

  it('renders all three tabs and identifies passive customer evidence', async () => {
    const wrapper = mountView()
    await flushPromises()

    expect(wrapper.get('[data-testid="tab-overview"]').exists()).toBe(true)
    expect(wrapper.get('[data-testid="tab-config"]').exists()).toBe(true)
    expect(wrapper.get('[data-testid="tab-history"]').exists()).toBe(true)
    expect(wrapper.text()).toContain('deepseek-v4-flash-0731')
    expect(wrapper.text()).toContain('admin.upstreamPriceMonitor.source.user_request')
    expect(wrapper.text()).toContain('$0.18')
    expect(wrapper.text()).toContain('$0.192')
  })

  it('creates an explicit dry run and never applies it implicitly', async () => {
    const wrapper = mountView()
    await flushPromises()

    await wrapper.get('[data-testid="run-now"]').trigger('click')
    await flushPromises()

    expect(api.createRun).toHaveBeenCalledWith({ dry_run: true })
    expect(api.applyRun).not.toHaveBeenCalled()
    wrapper.unmount()
  })

  it('requires confirmation and sends the snapshot hash when applying', async () => {
    const wrapper = mountView()
    await flushPromises()

    await wrapper.get('[data-testid="apply-latest"]').trigger('click')
    expect(wrapper.get('[data-testid="confirm-dialog"]').exists()).toBe(true)
    await wrapper.get('[data-testid="confirm-action"]').trigger('click')
    await flushPromises()

    expect(api.applyRun).toHaveBeenCalledWith(1, { snapshot_hash: 'abcdef1234567890' })
    wrapper.unmount()
  })

  it('saves account, channel and domestic-model rules without credentials', async () => {
    const wrapper = mountView()
    await flushPromises()
    await wrapper.get('[data-testid="tab-config"]').trigger('click')
    await wrapper.get('[data-testid="config-mode"]').setValue('auto_apply')
    await flushPromises()
    await wrapper.get('[data-testid="config-panel"] form').trigger('submit')
    await flushPromises()

    expect(api.updateConfig).toHaveBeenCalledWith(expect.objectContaining({
      mode: 'auto_apply',
      account_ids: [7],
      channel_ids: [3],
      domestic_models: ['deepseek-v4-flash-0731'],
    }))
    expect(JSON.stringify(api.updateConfig.mock.calls[0]?.[0])).not.toContain('api_key')
    wrapper.unmount()
  })

  it('manages a newly discovered model without deleting channel pricing', async () => {
    const wrapper = mountView()
    await flushPromises()
    await wrapper.get('[data-testid="tab-config"]').trigger('click')
    await wrapper.get('[data-testid="manage-model-deepseek-new-v1"]').trigger('click')
    await flushPromises()

    expect(api.updateModelStatus).toHaveBeenCalledWith('deepseek-new-v1', 'managed')
    expect(api.updateConfig).not.toHaveBeenCalled()
    wrapper.unmount()
  })

  it('refreshes the supported-model catalogue without starting a paid run', async () => {
    const wrapper = mountView()
    await flushPromises()
    await wrapper.get('[data-testid="tab-config"]').trigger('click')
    await wrapper.get('[data-testid="discover-models"]').trigger('click')
    await flushPromises()

    expect(api.discoverModels).toHaveBeenCalledTimes(1)
    expect(api.createRun).not.toHaveBeenCalled()
    wrapper.unmount()
  })
})
