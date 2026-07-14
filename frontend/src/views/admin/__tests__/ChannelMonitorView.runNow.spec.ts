import { defineComponent, nextTick } from 'vue'
import { flushPromises, mount } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'

import ChannelMonitorView from '../ChannelMonitorView.vue'
import type { ChannelMonitor, RunNowResponse } from '@/api/admin/channelMonitor'

const { listMonitors, runNow } = vi.hoisted(() => ({
  listMonitors: vi.fn(),
  runNow: vi.fn(),
}))

vi.mock('@/api/admin', () => ({
  adminAPI: {
    channelMonitor: {
      list: listMonitors,
      runNow,
      update: vi.fn(),
      del: vi.fn(),
    },
  },
}))

vi.mock('@/stores/app', () => ({
  useAppStore: () => ({
    showError: vi.fn(),
    showSuccess: vi.fn(),
  }),
}))

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return {
    ...actual,
    useI18n: () => ({ t: (key: string) => key }),
  }
})

const monitors: ChannelMonitor[] = [1, 2].map((id) => ({
  id,
  name: `monitor-${id}`,
  provider: 'grok',
  api_mode: 'chat_completions',
  endpoint: 'https://example.test',
  api_key_masked: 'sk-***',
  primary_model: 'grok-4.5',
  extra_models: [],
  group_name: '',
  enabled: true,
  interval_seconds: 60,
  jitter_seconds: 0,
  last_checked_at: null,
  created_by: 1,
  created_at: '2026-07-14T00:00:00Z',
  updated_at: '2026-07-14T00:00:00Z',
  primary_status: '',
  primary_latency_ms: null,
  availability_7d: 0,
  extra_models_status: [],
  template_id: null,
  extra_headers: {},
  body_override_mode: 'off',
  body_override: null,
}))

const DataTableStub = defineComponent({
  props: {
    data: { type: Array, default: () => [] },
  },
  template: `
    <div>
      <div v-for="row in data" :key="row.id">
        <slot name="cell-actions" :row="row" />
      </div>
    </div>
  `,
})

function mountView() {
  return mount(ChannelMonitorView, {
    global: {
      stubs: {
        AppLayout: { template: '<div><slot /></div>' },
        TablePageLayout: {
          template: '<div><slot name="filters" /><slot name="table" /><slot name="pagination" /></div>',
        },
        DataTable: DataTableStub,
        MonitorFiltersBar: true,
        MonitorFormDialog: true,
        MonitorTemplateManagerDialog: true,
        MonitorRunResultDialog: true,
        ConfirmDialog: true,
        Pagination: true,
        Icon: true,
      },
    },
  })
}

describe('admin ChannelMonitorView manual run', () => {
  beforeEach(() => {
    listMonitors.mockReset().mockResolvedValue({
      items: monitors,
      total: monitors.length,
      page: 1,
      page_size: 20,
      pages: 1,
    })
    runNow.mockReset()
  })

  it('sends one request for rapid clicks and disables every run button while pending', async () => {
    let resolveFirst!: (value: RunNowResponse) => void
    const firstRequest = new Promise<RunNowResponse>((resolve) => {
      resolveFirst = resolve
    })
    runNow.mockImplementationOnce(() => firstRequest).mockResolvedValue({ results: [] })

    const wrapper = mountView()
    await flushPromises()

    const first = wrapper.get('[data-testid="monitor-run-now-1"]')
    first.element.dispatchEvent(new MouseEvent('click'))
    first.element.dispatchEvent(new MouseEvent('click'))

    expect(runNow).toHaveBeenCalledTimes(1)
    expect(runNow).toHaveBeenLastCalledWith(1)
    await nextTick()
    expect(wrapper.get('[data-testid="monitor-run-now-1"]').attributes('disabled')).toBeDefined()
    expect(wrapper.get('[data-testid="monitor-run-now-2"]').attributes('disabled')).toBeDefined()

    wrapper.get('[data-testid="monitor-run-now-2"]').element.dispatchEvent(new MouseEvent('click'))
    expect(runNow).toHaveBeenCalledTimes(1)

    resolveFirst({ results: [] })
    await flushPromises()
    expect(wrapper.get('[data-testid="monitor-run-now-1"]').attributes('disabled')).toBeUndefined()
    expect(wrapper.get('[data-testid="monitor-run-now-2"]').attributes('disabled')).toBeUndefined()

    await wrapper.get('[data-testid="monitor-run-now-2"]').trigger('click')
    await flushPromises()
    expect(runNow).toHaveBeenCalledTimes(2)
    expect(runNow).toHaveBeenLastCalledWith(2)
  })
})
