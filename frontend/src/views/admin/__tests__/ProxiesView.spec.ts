import { computed, defineComponent, ref } from 'vue'
import { flushPromises, mount } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'

import ProxiesView from '../ProxiesView.vue'

const {
  listProxiesMock,
  getSettingsMock,
  updateSettingsMock
} = vi.hoisted(() => ({
  listProxiesMock: vi.fn(),
  getSettingsMock: vi.fn(),
  updateSettingsMock: vi.fn()
}))

const showError = vi.fn()
const showSuccess = vi.fn()
const showInfo = vi.fn()

vi.mock('@/api/admin', () => ({
  adminAPI: {
    proxies: {
      list: listProxiesMock,
      create: vi.fn(),
      update: vi.fn(),
      delete: vi.fn(),
      batchDelete: vi.fn(),
      testProxy: vi.fn(),
      checkQuality: vi.fn(),
      getProxyAccounts: vi.fn(),
      batchCreate: vi.fn()
    },
    settings: {
      getSettings: getSettingsMock,
      updateSettings: updateSettingsMock
    }
  }
}))

vi.mock('@/stores/app', () => ({
  useAppStore: () => ({
    showError,
    showSuccess,
    showInfo
  })
}))

vi.mock('@/composables/useClipboard', () => ({
  useClipboard: () => ({
    copyToClipboard: vi.fn()
  })
}))

vi.mock('@/composables/useSwipeSelect', () => ({
  useSwipeSelect: vi.fn()
}))

vi.mock('@/composables/useTableSelection', () => ({
  useTableSelection: () => ({
    selectedSet: ref(new Set<number>()),
    selectedCount: computed(() => 0),
    allVisibleSelected: computed(() => false),
    isSelected: () => false,
    select: vi.fn(),
    deselect: vi.fn(),
    clear: vi.fn(),
    removeMany: vi.fn(),
    toggleVisible: vi.fn(),
    batchUpdate: vi.fn()
  })
}))

vi.mock('@/composables/usePersistedPageSize', () => ({
  getPersistedPageSize: () => 20
}))

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return {
    ...actual,
    useI18n: () => ({
      t: (key: string) => key
    })
  }
})

const DataTableStub = defineComponent({
  name: 'DataTableStub',
  props: {
    data: {
      type: Array,
      default: () => []
    }
  },
  template: `
    <div>
      <div v-for="row in data" :key="row.id">
        <slot name="cell-name" :row="row" :value="row.name" />
        <slot name="cell-actions" :row="row" />
      </div>
    </div>
  `
})

const TablePageLayoutStub = defineComponent({
  name: 'TablePageLayoutStub',
  template: '<div><slot name="filters" /><slot name="table" /><slot name="pagination" /></div>'
})

const baseSettings = {
  site_name: 'Sub2API',
  default_proxy_id: 1
}

const proxyRows = [
  {
    id: 1,
    name: 'Proxy A',
    protocol: 'http',
    host: 'a.example.com',
    port: 8080,
    username: null,
    password: null,
    status: 'active',
    account_count: 0,
    created_at: '2026-04-27T00:00:00Z',
    updated_at: '2026-04-27T00:00:00Z'
  },
  {
    id: 2,
    name: 'Proxy B',
    protocol: 'http',
    host: 'b.example.com',
    port: 8081,
    username: null,
    password: null,
    status: 'active',
    account_count: 0,
    created_at: '2026-04-27T00:00:00Z',
    updated_at: '2026-04-27T00:00:00Z'
  }
]

const createWrapper = () =>
  mount(ProxiesView, {
    global: {
      stubs: {
        AppLayout: { template: '<div><slot /></div>' },
        TablePageLayout: TablePageLayoutStub,
        DataTable: DataTableStub,
        Pagination: true,
        BaseDialog: true,
        ConfirmDialog: true,
        EmptyState: true,
        ImportDataModal: true,
        Select: true,
        Icon: true,
        PlatformTypeBadge: true,
        Teleport: true
      }
    }
  })

describe('ProxiesView', () => {
  beforeEach(() => {
    listProxiesMock.mockReset()
    getSettingsMock.mockReset()
    updateSettingsMock.mockReset()
    showError.mockReset()
    showSuccess.mockReset()
    showInfo.mockReset()

    listProxiesMock.mockResolvedValue({
      items: proxyRows,
      total: 2,
      page: 1,
      page_size: 20,
      pages: 1
    })
    getSettingsMock.mockResolvedValue(baseSettings)
    updateSettingsMock.mockResolvedValue({ ...baseSettings, default_proxy_id: 2 })
  })

  it('可以显示并切换默认代理', async () => {
    const wrapper = createWrapper()

    await flushPromises()

    expect(wrapper.text()).toContain('admin.proxies.defaultBadge')

    const setDefaultButton = wrapper
      .findAll('button')
      .find((button) => button.text().includes('admin.proxies.setAsDefault'))

    expect(setDefaultButton).toBeTruthy()
    await setDefaultButton!.trigger('click')
    await flushPromises()

    expect(updateSettingsMock).toHaveBeenCalledWith(
      expect.objectContaining({
        site_name: 'Sub2API',
        default_proxy_id: 2
      })
    )
    expect(showSuccess).toHaveBeenCalledWith('admin.proxies.defaultProxySet')
  })
})
