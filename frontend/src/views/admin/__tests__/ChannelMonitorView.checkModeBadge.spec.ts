import { defineComponent } from 'vue'
import { flushPromises, mount } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'

import type { ChannelMonitor } from '@/api/admin/channelMonitor'
import ChannelMonitorView from '@/views/admin/ChannelMonitorView.vue'

// P2-7(d)：provider 徽章旁并列 check_mode 徽章，三种检测模式一眼可分
// （quota 系 = 账号配额数据源，probe = 纯探活）。

const { listMonitors } = vi.hoisted(() => ({ listMonitors: vi.fn() }))

vi.mock('@/utils/featureFlags', () => ({
  isChannelMonitorV1Mode: () => true,
  isChannelMonitorV2Mode: () => false,
  getChannelMonitorMode: () => 'v1' as const,
}))

vi.mock('@/features/channel-monitor-v2/MonitorSettingsPanel.vue', () => ({
  default: { name: 'MonitorSettingsPanel', template: '<div data-testid="v2-settings" />' },
}))

vi.mock('@/api/admin', () => ({
  adminAPI: {
    channelMonitor: {
      list: listMonitors,
      duplicate: vi.fn(),
      update: vi.fn(),
      runNow: vi.fn(),
      del: vi.fn(),
    },
  },
}))

vi.mock('@/stores/app', () => ({
  useAppStore: () => ({ showSuccess: vi.fn(), showError: vi.fn() }),
}))

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return {
    ...actual,
    useI18n: () => ({ t: (key: string) => key }),
  }
})

const AppLayoutStub = defineComponent({
  template: '<main><slot /></main>',
})

const TablePageLayoutStub = defineComponent({
  template: '<section><slot name="filters" /><slot name="table" /><slot name="pagination" /></section>',
})

// 渲染 cell-provider slot，徽章断言基于它。
const DataTableStub = defineComponent({
  props: {
    data: { type: Array, default: () => [] },
    columns: { type: Array, default: () => [] },
    loading: { type: Boolean, default: false },
  },
  template: '<div><div v-for="row in data" :key="row.id" class="provider-cell"><slot name="cell-provider" :row="row" /></div></div>',
})

function makeMonitor(overrides: Partial<ChannelMonitor> = {}): ChannelMonitor {
  return {
    id: 42,
    name: 'primary',
    provider: 'openai',
    api_mode: 'chat_completions',
    endpoint: 'https://api.example.com',
    api_key_masked: 'sk-t***',
    primary_model: 'gpt-4o-mini',
    extra_models: [],
    group_name: '',
    enabled: true,
    interval_seconds: 60,
    jitter_seconds: 0,
    last_checked_at: null,
    created_by: 1,
    created_at: '2026-07-16T00:00:00Z',
    updated_at: '2026-07-16T00:00:00Z',
    primary_status: '',
    primary_latency_ms: null,
    availability_7d: 0,
    extra_models_status: [],
    template_id: null,
    extra_headers: {},
    body_override_mode: 'off',
    body_override: null,
    check_mode: 'probe',
    account_id: null,
    ...overrides,
  }
}

function mountView() {
  return mount(ChannelMonitorView, {
    global: {
      stubs: {
        AppLayout: AppLayoutStub,
        TablePageLayout: TablePageLayoutStub,
        DataTable: DataTableStub,
        MonitorFiltersBar: true,
        Pagination: true,
        ConfirmDialog: true,
        EmptyState: true,
        HelpTooltip: true,
        Toggle: true,
        MonitorFormDialog: true,
        MonitorTemplateManagerDialog: true,
        MonitorRunResultDialog: true,
        MonitorPrimaryModelCell: true,
      },
    },
  })
}

describe('ChannelMonitorView check-mode badge', () => {
  beforeEach(() => {
    localStorage.clear()
    listMonitors.mockReset()
  })

  it.each([
    ['quota', 'monitorCommon.checkMode.quota'],
    ['quota_probe', 'monitorCommon.checkMode.quota_probe'],
    ['probe', 'monitorCommon.checkMode.probe'],
  ] as const)('renders the %s badge next to the provider badge', async (mode, label) => {
    listMonitors.mockResolvedValue({
      items: [makeMonitor({ check_mode: mode })],
      total: 1,
      page: 1,
      page_size: 20,
      pages: 1,
    })
    const wrapper = mountView()
    await flushPromises()

    const cell = wrapper.get('.provider-cell')
    expect(cell.text()).toContain(label)
    // provider 徽章仍在（不因并列展示被顶掉）。
    expect(cell.text()).toContain('monitorCommon.providers.openai')

    const badges = cell.findAll('span')
    const modeBadge = badges.find(el => el.text() === label)
    expect(modeBadge).toBeDefined()
    // quota 系=蓝、probe=中性灰的配色区分。
    const cls = modeBadge!.attributes('class')
    if (mode === 'probe') {
      expect(cls).toContain('bg-gray-100')
      expect(cls).not.toContain('bg-blue-100')
    } else {
      expect(cls).toContain('bg-blue-100')
      expect(cls).not.toContain('bg-gray-100')
    }
    wrapper.unmount()
  })
})
