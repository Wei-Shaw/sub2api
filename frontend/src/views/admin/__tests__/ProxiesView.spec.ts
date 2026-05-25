import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'

const { list, batchCreate } = vi.hoisted(() => {
  vi.stubGlobal('localStorage', {
    getItem: vi.fn(() => null),
    setItem: vi.fn(),
    removeItem: vi.fn()
  })

  return {
    list: vi.fn(),
    batchCreate: vi.fn()
  }
})

import ProxiesView from '../ProxiesView.vue'

const showError = vi.fn()
const showSuccess = vi.fn()
const showInfo = vi.fn()

vi.mock('@/api/admin', () => ({
  adminAPI: {
    proxies: {
      list,
      batchCreate,
      create: vi.fn(),
      update: vi.fn(),
      delete: vi.fn(),
      batchDelete: vi.fn(),
      testProxy: vi.fn(),
      checkProxyQuality: vi.fn(),
      exportData: vi.fn(),
      getProxyAccounts: vi.fn()
    }
  }
}))

vi.mock('@/stores/app', () => ({
  useAppStore: () => ({
    showError,
    showSuccess,
    showInfo,
    showWarning: vi.fn()
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

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return {
    ...actual,
    useI18n: () => ({
      t: (key: string) => key
    })
  }
})

const globalStubs = {
  AppLayout: { template: '<div><slot /></div>' },
  TablePageLayout: { template: '<div><slot name="filters" /><slot name="table" /></div>' },
  DataTable: { template: '<div />' },
  BaseDialog: { template: '<div><slot /><slot name="footer" /></div>' },
  Select: { template: '<div />' },
  Icon: true,
  ConfirmDialog: true,
  ProxyExportDialog: true,
  ImportDataModal: true,
  Pagination: true,
  PlatformTypeBadge: true
}

describe('admin ProxiesView batch add', () => {
  beforeEach(() => {
    list.mockReset()
    batchCreate.mockReset()
    showError.mockReset()
    showSuccess.mockReset()
    showInfo.mockReset()

    list.mockResolvedValue({
      items: [],
      total: 0,
      pages: 0
    })
    batchCreate.mockResolvedValue({
      created: 1,
      skipped: 0
    })
  })

  it('requires a batch name prefix before importing proxies', async () => {
    const wrapper = mount(ProxiesView, {
      global: {
        stubs: globalStubs
      }
    })

    await flushPromises()

    const buttons = wrapper.findAll('button')
    await buttons.find((button) => button.text().includes('admin.proxies.batchAdd'))?.trigger('click')
    const textarea = wrapper.find('textarea')
    await textarea.setValue('http://10.0.0.1:8080')
    await wrapper.findAll('button').find((button) => button.text().includes('admin.proxies.importProxies'))?.trigger('click')

    expect(showError).toHaveBeenCalledWith('admin.proxies.batchNamePrefixRequired')
    expect(batchCreate).not.toHaveBeenCalled()
  })

  it('sends name_prefix in batch create payload', async () => {
    const wrapper = mount(ProxiesView, {
      global: {
        stubs: globalStubs
      }
    })

    await flushPromises()

    const buttons = wrapper.findAll('button')
    await buttons.find((button) => button.text().includes('admin.proxies.batchAdd'))?.trigger('click')

    const inputs = wrapper.findAll('input[type="text"]')
    const prefixInput = inputs.find((input) => input.attributes('placeholder') === 'admin.proxies.batchNamePrefixPlaceholder')
    expect(prefixInput).toBeTruthy()
    await prefixInput!.setValue('tokyo')

    const textarea = wrapper.find('textarea')
    await textarea.setValue('http://10.0.0.1:8080')

    await wrapper.findAll('button').find((button) => button.text().includes('admin.proxies.importProxies'))?.trigger('click')
    await flushPromises()

    expect(batchCreate).toHaveBeenCalledWith({
      name_prefix: 'tokyo',
      proxies: [
        {
          protocol: 'http',
          host: '10.0.0.1',
          port: 8080,
          username: '',
          password: ''
        }
      ]
    })
  })
})
