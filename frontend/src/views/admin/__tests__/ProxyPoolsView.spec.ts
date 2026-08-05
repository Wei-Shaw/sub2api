import { flushPromises, mount } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { defineComponent, h } from 'vue'

import ProxyPoolsView from '../ProxyPoolsView.vue'

const {
  listPools,
  listProxies,
  rebindLogs,
  rebind,
  showError,
  showSuccess
} = vi.hoisted(() => ({
  listPools: vi.fn(),
  listProxies: vi.fn(),
  rebindLogs: vi.fn(),
  rebind: vi.fn(),
  showError: vi.fn(),
  showSuccess: vi.fn()
}))

vi.mock('@/api/admin', () => ({
  adminAPI: {
    proxyPools: {
      list: listPools,
      listProxies,
      rebindLogs,
      rebind,
      create: vi.fn(),
      update: vi.fn(),
      remove: vi.fn(),
      assignProxies: vi.fn(),
      removeProxies: vi.fn()
    },
    proxies: {
      getAll: vi.fn()
    }
  }
}))

vi.mock('@/stores/app', () => ({
  useAppStore: () => ({ showError, showSuccess })
}))

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return {
    ...actual,
    useI18n: () => ({ t: (key: string) => key })
  }
})

const DataTableStub = defineComponent({
  props: {
    data: { type: Array, default: () => [] }
  },
  setup(props, { slots }) {
    return () => h('div', { class: 'data-table-stub' }, (props.data as Record<string, unknown>[]).map((row) =>
      h('div', { class: 'data-row' }, [
        slots['cell-name']?.({ row, value: row.name }),
        slots['cell-actions']?.({ row })
      ])
    ))
  }
})

function makePool() {
  return {
    id: 1,
    name: 'primary-pool',
    description: 'Primary exits',
    status: 'active',
    health_interval_seconds: 300,
    failure_threshold: 2,
    auto_rebind: true,
    proxy_count: 2,
    healthy_proxy_count: 1,
    unhealthy_proxy_count: 1,
    bound_account_count: 4,
    created_at: '2026-08-04T00:00:00Z',
    updated_at: '2026-08-04T00:00:00Z'
  }
}

function mountView() {
  return mount(ProxyPoolsView, {
    global: {
      stubs: {
        AppLayout: { template: '<div><slot /></div>' },
        TablePageLayout: { template: '<div><slot name="filters" /><slot name="table" /></div>' },
        DataTable: DataTableStub,
        BaseDialog: {
          props: ['show', 'title'],
          template: '<section v-if="show" class="dialog-stub"><slot /><slot name="footer" /></section>'
        },
        ConfirmDialog: true,
        Select: true,
        Icon: true
      }
    }
  })
}

describe('ProxyPoolsView', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    listPools.mockResolvedValue([makePool()])
    listProxies.mockResolvedValue([
      {
        id: 11,
        name: 'exit-a',
        protocol: 'http',
        host: 'proxy.example',
        port: 8080,
        status: 'active',
        pool_id: 1,
        pool_health: 'healthy',
        pool_failures: 0,
        pool_checked_at: '2026-08-04T00:00:00Z',
        account_count: 4,
        latency_ms: 42,
        ip_address: '203.0.113.25',
        country: 'Example',
        country_code: 'EX'
      }
    ])
    rebindLogs.mockResolvedValue([])
    rebind.mockResolvedValue(2)
  })

  it('loads proxy pools on mount', async () => {
    const wrapper = mountView()
    await flushPromises()

    expect(listPools).toHaveBeenCalledTimes(1)
    expect(wrapper.text()).toContain('primary-pool')
  })

  it('shows cached exit information in pool details', async () => {
    const wrapper = mountView()
    await flushPromises()

    await wrapper.get('button[title="admin.proxyPools.details"]').trigger('click')
    await flushPromises()

    expect(listProxies).toHaveBeenCalledWith(1)
    expect(rebindLogs).toHaveBeenCalledWith(1)
    expect(wrapper.text()).toContain('203.0.113.25')
    expect(wrapper.text()).toContain('42ms')
    expect(wrapper.text()).toContain('Example')
  })

  it('runs a forced health check and refreshes the detail', async () => {
    const wrapper = mountView()
    await flushPromises()
    await wrapper.get('button[title="admin.proxyPools.details"]').trigger('click')
    await flushPromises()

    const checkButton = wrapper.findAll('button').find((button) => button.text().includes('admin.proxyPools.checkNow'))
    expect(checkButton).toBeDefined()
    await checkButton!.trigger('click')
    await flushPromises()

    expect(rebind).toHaveBeenCalledWith(1)
    expect(listProxies).toHaveBeenCalledTimes(2)
    expect(showSuccess).toHaveBeenCalled()
  })
})
