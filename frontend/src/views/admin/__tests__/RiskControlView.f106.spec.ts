import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import type { VueWrapper } from '@vue/test-utils'

import RiskControlView from '../RiskControlView.vue'
import type { ContentModerationConfig, UpdateContentModerationConfig } from '@/api/admin/riskControl'

const f106Mocks = vi.hoisted(() => ({
  getConfig: vi.fn(),
  updateConfig: vi.fn(),
  getStatus: vi.fn(),
  listLogs: vi.fn(),
  getGroups: vi.fn(),
  getProxies: vi.fn(),
  showError: vi.fn(),
  showSuccess: vi.fn(),
}))

vi.mock('@/api/admin', () => ({
  adminAPI: {
    riskControl: {
      getConfig: f106Mocks.getConfig,
      updateConfig: f106Mocks.updateConfig,
      getStatus: f106Mocks.getStatus,
      listLogs: f106Mocks.listLogs,
      testAPIKeys: vi.fn(),
      deleteFlaggedHash: vi.fn(),
      clearFlaggedHashes: vi.fn(),
      unbanUser: vi.fn(),
    },
    groups: { getAll: f106Mocks.getGroups },
    proxies: { getAll: f106Mocks.getProxies },
  },
}))

vi.mock('@/stores/app', () => ({
  useAppStore: () => ({ showError: f106Mocks.showError, showSuccess: f106Mocks.showSuccess }),
}))

vi.mock('@/utils/apiError', () => ({
  extractApiErrorMessage: (_err: unknown, fallback: string) => fallback,
}))

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return {
    ...actual,
    useI18n: () => ({
      t: (key: string, params?: Record<string, string | number>) =>
        key.replace(/\{(\w+)\}/g, (_, token) => String(params?.[token] ?? `{${token}}`)),
    }),
  }
})

const f106BaseConfig = (): ContentModerationConfig => ({
  enabled: true,
  mode: 'pre_block',
  base_url: 'https://api.openai.com',
  model: 'omni-moderation-latest',
  proxy_id: null,
  api_key_configured: false,
  api_key_masked: '',
  api_key_count: 0,
  api_key_masks: [],
  api_key_statuses: [],
  timeout_ms: 3000,
  sample_rate: 100,
  all_groups: true,
  group_ids: [],
  record_non_hits: false,
  worker_count: 4,
  queue_size: 32768,
  block_status: 403,
  block_message: 'blocked',
  email_on_hit: true,
  auto_ban_enabled: true,
  ban_threshold: 10,
  violation_window_hours: 720,
  retry_count: 2,
  hit_retention_days: 180,
  non_hit_retention_days: 3,
  pre_hash_check_enabled: false,
  blocked_keywords: [],
  keyword_blocking_mode: 'keyword_and_api',
  thresholds: { harassment: 0.98, sexual: 0.65 },
  model_filter: { type: 'all', models: [] },
})

const f106Stubs = {
  AppLayout: { template: '<div><slot /></div>' },
  BaseDialog: { props: { show: { type: Boolean, default: false } }, template: '<div v-if="show"><slot /><slot name="footer" /></div>' },
  Icon: true,
  Select: true,
  Toggle: true,
  Pagination: true,
  ModelWhitelistSelector: true,
  ProxySelector: true,
}

async function f106ClickButton(wrapper: VueWrapper, text: string) {
  const button = wrapper.findAll('button').find((item) => item.text().includes(text))
  if (!button) throw new Error(`button not found: ${text}`)
  await button.trigger('click')
}

async function f106SaveWithSampleRate(value: string) {
  const wrapper = mount(RiskControlView, { global: { stubs: f106Stubs } })
  await flushPromises()
  await f106ClickButton(wrapper, 'admin.riskControl.openSettings')
  await wrapper.get('input[type="number"][min="0"][max="100"][step="1"]').setValue(value)
  await f106ClickButton(wrapper, 'admin.riskControl.saveConfig')
  await flushPromises()
  wrapper.unmount()
}

describe('RiskControlView sample_rate save (F1-06)', () => {
  beforeEach(() => {
    Object.values(f106Mocks).forEach((mock) => mock.mockReset())
    f106Mocks.getConfig.mockResolvedValue(f106BaseConfig())
    f106Mocks.getStatus.mockResolvedValue({ enabled: true, worker_count: 4, api_key_statuses: [] })
    f106Mocks.listLogs.mockResolvedValue({ items: [], total: 0, page: 1, page_size: 20, pages: 1 })
    f106Mocks.getGroups.mockResolvedValue([])
    f106Mocks.getProxies.mockResolvedValue([])
    f106Mocks.updateConfig.mockImplementation(async (payload: UpdateContentModerationConfig) => ({
      ...f106BaseConfig(),
      ...payload,
      model_filter: payload.model_filter ?? f106BaseConfig().model_filter,
    }))
  })

  it('falls back to the default 100 when the sample rate input is cleared', async () => {
    await f106SaveWithSampleRate('')
    expect(f106Mocks.updateConfig).toHaveBeenCalledWith(expect.objectContaining({ sample_rate: 100 }))
  })

  it('keeps an explicit 0 sample rate', async () => {
    await f106SaveWithSampleRate('0')
    expect(f106Mocks.updateConfig).toHaveBeenCalledWith(expect.objectContaining({ sample_rate: 0 }))
  })

  it('keeps an explicit non-default sample rate', async () => {
    await f106SaveWithSampleRate('35')
    expect(f106Mocks.updateConfig).toHaveBeenCalledWith(expect.objectContaining({ sample_rate: 35 }))
  })
})
