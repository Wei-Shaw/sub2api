import { describe, it, expect, vi, beforeEach } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import ImportDataModal from '@/components/admin/account/ImportDataModal.vue'
import { adminAPI } from '@/api/admin'

const showError = vi.fn()
const showSuccess = vi.fn()
const scrollTo = vi.fn()
const waitForImport = async () => {
  for (let i = 0; i < 10; i++) {
    await flushPromises()
    await new Promise((resolve) => setTimeout(resolve, 0))
  }
}

vi.mock('@/stores/app', () => ({
  useAppStore: () => ({
    showError,
    showSuccess
  })
}))

vi.mock('@/api/admin', () => ({
    adminAPI: {
      accounts: {
        importData: vi.fn(),
        inspectData: vi.fn(),
        inspectDataStream: vi.fn()
      },
    groups: {
      create: vi.fn()
    }
  }
}))

vi.mock('vue-i18n', () => ({
  useI18n: () => ({
    t: (key: string, params?: Record<string, unknown>) => {
      if (key === 'admin.accounts.dataImportImportedCount') {
        return `${key}:${params?.count ?? ''}`
      }
      if (key === 'admin.accounts.dataImportConfirmImport') {
        return `${key}:${params?.count ?? ''}`
      }
      if ([
        'admin.accounts.dataImportProgressSkipped',
        'admin.accounts.dataImportProgressAbnormal',
        'admin.accounts.dataImportProgressImported',
        'admin.accounts.dataImportProgressHealthy',
        'admin.accounts.dataImportProgressScanned'
      ].includes(key)) {
        return `${key}:${params?.count ?? ''}`
      }
      return key
    }
  })
}))

const openAIGroup = (id: number, name: string) => ({
  id,
  name,
  description: null,
  platform: 'openai' as const,
  rate_multiplier: 1,
  is_exclusive: false,
  status: 'active' as const,
  subscription_type: 'standard' as const,
  daily_limit_usd: null,
  weekly_limit_usd: null,
  monthly_limit_usd: null,
  allow_image_generation: false,
  image_rate_independent: false,
  image_rate_multiplier: 1,
  image_price_1k: null,
  image_price_2k: null,
  image_price_4k: null,
  claude_code_only: false,
  fallback_group_id: null,
  fallback_group_id_on_invalid_request: null,
  require_oauth_only: false,
  require_privacy_set: false,
  created_at: '2026-01-01T00:00:00Z',
  updated_at: '2026-01-01T00:00:00Z'
})

describe('ImportDataModal', () => {
  beforeEach(() => {
    showError.mockReset()
    showSuccess.mockReset()
    scrollTo.mockReset()
    Object.defineProperty(HTMLElement.prototype, 'scrollTo', {
      configurable: true,
      value: scrollTo
    })
    vi.mocked(adminAPI.accounts.importData).mockReset()
    vi.mocked(adminAPI.accounts.inspectData).mockReset()
    vi.mocked(adminAPI.accounts.inspectDataStream).mockReset()
    vi.mocked(adminAPI.groups.create).mockReset()
  })

  it('未选择文件时提示错误', async () => {
    const wrapper = mount(ImportDataModal, {
      props: { show: true },
      global: {
        stubs: {
          BaseDialog: { template: '<div><slot /><slot name="footer" /></div>' }
        }
      }
    })

    await wrapper.find('form').trigger('submit')
    expect(showError).toHaveBeenCalledWith('admin.accounts.dataImportSelectFile')
  })

  it('无效 JSON 时提示解析失败', async () => {
    const wrapper = mount(ImportDataModal, {
      props: { show: true },
      global: {
        stubs: {
          BaseDialog: { template: '<div><slot /><slot name="footer" /></div>' }
        }
      }
    })

    const input = wrapper.find('input[type="file"]')
    const file = new File(['invalid json'], 'data.json', { type: 'application/json' })
    Object.defineProperty(file, 'text', {
      value: () => Promise.resolve('invalid json')
    })
    Object.defineProperty(input.element, 'files', {
      value: [file]
    })

    await input.trigger('change')
    await wrapper.find('form').trigger('submit')
    await waitForImport()

    expect(showError).toHaveBeenCalledWith('admin.accounts.dataImportParseFailed')
  })

  it('巡查开启时只导入健康账号并带上分组', async () => {
    vi.mocked(adminAPI.accounts.inspectDataStream).mockImplementation(async (_payload, onEvent) => {
      onEvent({
        type: 'item',
        item: { index: 0, name: 'healthy', platform: 'openai', type: 'apikey', healthy: true }
      })
      onEvent({
        type: 'item',
        item: { index: 1, name: 'bad', platform: 'openai', type: 'apikey', healthy: false, reasons: ['api_key is required'] }
      })
      onEvent({
        type: 'done',
        result: {
          total: 2,
          healthy: 1,
          unhealthy: 1,
          valid_proxy_keys: [],
          results: [
            { index: 0, name: 'healthy', platform: 'openai', type: 'apikey', healthy: true },
            { index: 1, name: 'bad', platform: 'openai', type: 'apikey', healthy: false, reasons: ['api_key is required'] }
          ]
        }
      })
    })
    vi.mocked(adminAPI.accounts.inspectData).mockResolvedValue({
      total: 2,
      healthy: 1,
      unhealthy: 1,
      valid_proxy_keys: [],
      results: [
        { index: 0, name: 'healthy', platform: 'openai', type: 'apikey', healthy: true },
        { index: 1, name: 'bad', platform: 'openai', type: 'apikey', healthy: false, reasons: ['api_key is required'] }
      ]
    })
    vi.mocked(adminAPI.accounts.importData).mockResolvedValue({
      proxy_created: 0,
      proxy_reused: 0,
      proxy_failed: 0,
      account_created: 1,
      account_failed: 0
    })

    const wrapper = mount(ImportDataModal, {
      props: {
        show: true,
        groups: [openAIGroup(7, 'openai-default'), openAIGroup(8, 'openai-extra')]
      },
      global: {
        stubs: {
          BaseDialog: { template: '<div><slot /><slot name="footer" /></div>' }
        }
      }
    })

    const payload = {
      exported_at: '2026-01-01T00:00:00Z',
      proxies: [],
      accounts: [
        {
          name: 'healthy',
          platform: 'openai',
          type: 'apikey',
          credentials: { api_key: 'sk-test' },
          concurrency: 1,
          priority: 50
        },
        {
          name: 'bad',
          platform: 'openai',
          type: 'apikey',
          credentials: { email: 'bad@example.com' },
          concurrency: 1,
          priority: 50
        }
      ]
    }

    const input = wrapper.find('input[type="file"]')
    const file = new File([JSON.stringify(payload)], 'data.json', { type: 'application/json' })
    Object.defineProperty(file, 'text', {
      value: () => Promise.resolve(JSON.stringify(payload))
    })
    Object.defineProperty(input.element, 'files', {
      value: [file]
    })

    await input.trigger('change')
    const firstGroupInput = wrapper.find('input[name="import-data-group"][value="7"]')
    const secondGroupInput = wrapper.find('input[name="import-data-group"][value="8"]')
    expect(firstGroupInput.attributes('type')).toBe('checkbox')
    expect(firstGroupInput.classes()).toContain('rounded')
    await firstGroupInput.setValue(true)
    await secondGroupInput.setValue(true)
    await wrapper.find('form').trigger('submit')
    await waitForImport()

    expect(adminAPI.accounts.inspectDataStream).toHaveBeenCalledTimes(1)
    expect(adminAPI.accounts.inspectData).not.toHaveBeenCalled()
    expect(adminAPI.accounts.importData).not.toHaveBeenCalled()
    expect(showSuccess).not.toHaveBeenCalledWith('admin.accounts.dataImportImportedCount:1')
    expect(wrapper.text()).toContain('admin.accounts.dataImportConfirmImport:1')
    expect(wrapper.text()).toContain('admin.accounts.dataImportProgressAbnormal:1')
    expect(wrapper.text()).not.toContain('admin.accounts.dataImportProgressSkipped:1')
    expect(wrapper.text()).not.toContain('admin.accounts.dataImportProgressImported:0')
    expect(wrapper.text()).not.toContain('admin.accounts.dataImportResult')
    expect(wrapper.text()).not.toContain('admin.accounts.dataImportResultSummary')
    expect(wrapper.text()).toContain('bad@example.com')
    expect(wrapper.text()).toContain('api_key is required')
    expect(showError).not.toHaveBeenCalledWith('admin.accounts.dataImportCompletedWithErrors')

    await wrapper.find('form').trigger('submit')
    await waitForImport()

    expect(adminAPI.accounts.importData).toHaveBeenCalledTimes(1)
    expect(adminAPI.accounts.importData).toHaveBeenCalledWith({
      data: {
        ...payload,
        accounts: [payload.accounts[0]]
      },
      group_ids: [8],
      skip_default_group_bind: true
    })
    expect(showSuccess).toHaveBeenCalledWith('admin.accounts.dataImportImportedCount:1')
    expect(wrapper.text()).not.toContain('admin.accounts.dataImportResult')
    expect(wrapper.text()).not.toContain('admin.accounts.dataImportResultSummary')
  })

  it('大批量导入时分块巡查和导入，并避免重复携带代理列表', async () => {
    vi.mocked(adminAPI.accounts.inspectDataStream).mockImplementation(async ({ data, valid_proxy_keys }, onEvent) => {
      const result = {
        total: data.accounts.length,
        healthy: data.accounts.length,
        unhealthy: 0,
        valid_proxy_keys: Array.from(new Set([...(valid_proxy_keys || []), 'http|proxy.local|8080||'])),
        results: data.accounts.map((account, index) => ({
          index,
          name: account.name,
          platform: account.platform,
          type: account.type,
          healthy: true
        }))
      }
      for (const item of result.results) {
        onEvent({ type: 'item', item })
      }
      onEvent({ type: 'done', result })
    })
    vi.mocked(adminAPI.accounts.importData).mockImplementation(async ({ data }) => ({
      proxy_created: data.proxies.length,
      proxy_reused: 0,
      proxy_failed: 0,
      account_created: data.accounts.length,
      account_failed: 0
    }))

    const wrapper = mount(ImportDataModal, {
      props: { show: true },
      global: {
        stubs: {
          BaseDialog: { template: '<div><slot /><slot name="footer" /></div>' }
        }
      }
    })

    const accounts = Array.from({ length: 401 }, (_, index) => ({
      name: `account-${index}`,
      platform: 'openai' as const,
      type: 'apikey' as const,
      credentials: { api_key: `sk-${index}` },
      proxy_key: 'http|proxy.local|8080||',
      concurrency: 1,
      priority: 50
    }))
    const payload = {
      exported_at: '2026-01-01T00:00:00Z',
      proxies: [
        {
          proxy_key: 'http|proxy.local|8080||',
          name: 'proxy',
          protocol: 'http' as const,
          host: 'proxy.local',
          port: 8080,
          status: 'active' as const
        }
      ],
      accounts
    }

    const input = wrapper.find('input[type="file"]')
    const file = new File([JSON.stringify(payload)], 'bulk.json', { type: 'application/json' })
    Object.defineProperty(file, 'text', {
      value: () => Promise.resolve(JSON.stringify(payload))
    })
    Object.defineProperty(input.element, 'files', {
      value: [file]
    })

    await input.trigger('change')
    await wrapper.find('form').trigger('submit')
    await waitForImport()

    expect(adminAPI.accounts.inspectDataStream).toHaveBeenCalledTimes(5)
    expect(adminAPI.accounts.importData).not.toHaveBeenCalled()
    expect(showSuccess).not.toHaveBeenCalledWith('admin.accounts.dataImportImportedCount:401')
    expect(wrapper.text()).toContain('admin.accounts.dataImportConfirmImport:401')

    await wrapper.find('form').trigger('submit')
    await waitForImport()

    expect(adminAPI.accounts.importData).toHaveBeenCalledTimes(3)
    expect(vi.mocked(adminAPI.accounts.inspectDataStream).mock.calls[0][0].data.proxies).toHaveLength(1)
    expect(vi.mocked(adminAPI.accounts.inspectDataStream).mock.calls[1][0].data.proxies).toHaveLength(1)
    expect(vi.mocked(adminAPI.accounts.inspectDataStream).mock.calls[1][0].valid_proxy_keys).toContain('http|proxy.local|8080||')
    expect(vi.mocked(adminAPI.accounts.importData).mock.calls[0][0].data.accounts).toHaveLength(200)
    expect(vi.mocked(adminAPI.accounts.importData).mock.calls[1][0].data.accounts).toHaveLength(200)
    expect(vi.mocked(adminAPI.accounts.importData).mock.calls[2][0].data.accounts).toHaveLength(1)
    expect(vi.mocked(adminAPI.accounts.importData).mock.calls[0][0].data.proxies).toHaveLength(1)
    expect(vi.mocked(adminAPI.accounts.importData).mock.calls[1][0].data.proxies).toHaveLength(0)
    expect(showSuccess).toHaveBeenCalledWith('admin.accounts.dataImportImportedCount:401')
  })

  it('新建分组同步到父列表后不重复显示', async () => {
    const createdGroup = openAIGroup(9, 'new-openai')
    vi.mocked(adminAPI.groups.create).mockResolvedValue(createdGroup)

    const wrapper = mount(ImportDataModal, {
      props: { show: true, groups: [] },
      global: {
        stubs: {
          BaseDialog: { template: '<div><slot /><slot name="footer" /></div>' },
          Icon: { template: '<span />' }
        }
      }
    })

    await wrapper.find('button.btn-secondary.btn-sm').trigger('click')
    await wrapper.find('input[type="text"]').setValue('new-openai')
    await wrapper.find('button.btn-primary.w-full').trigger('click')
    await flushPromises()

    await wrapper.setProps({ groups: [createdGroup] })
    await flushPromises()

    const renderedNewGroups = wrapper
      .findAll('label')
      .filter((label) => label.text().includes('new-openai'))
    expect(renderedNewGroups).toHaveLength(1)
    expect(wrapper.find('input[name="import-data-group"][value="9"]').element.checked).toBe(true)
  })

  it('can cancel after inspection without importing', async () => {
    vi.mocked(adminAPI.accounts.inspectDataStream).mockImplementation(async (_payload, onEvent) => {
      onEvent({
        type: 'item',
        item: { index: 0, name: 'healthy', platform: 'openai', type: 'apikey', healthy: true }
      })
      onEvent({
        type: 'done',
        result: {
          total: 1,
          healthy: 1,
          unhealthy: 0,
          valid_proxy_keys: [],
          results: [
            { index: 0, name: 'healthy', platform: 'openai', type: 'apikey', healthy: true }
          ]
        }
      })
    })

    const wrapper = mount(ImportDataModal, {
      props: { show: true },
      global: {
        stubs: {
          BaseDialog: { template: '<div><slot /><slot name="footer" /></div>' }
        }
      }
    })

    const payload = {
      exported_at: '2026-01-01T00:00:00Z',
      proxies: [],
      accounts: [
        {
          name: 'healthy',
          platform: 'openai',
          type: 'apikey',
          credentials: { api_key: 'sk-ok' },
          concurrency: 1,
          priority: 50
        }
      ]
    }

    const input = wrapper.find('input[type="file"]')
    const file = new File([JSON.stringify(payload)], 'cancel.json', { type: 'application/json' })
    Object.defineProperty(file, 'text', {
      value: () => Promise.resolve(JSON.stringify(payload))
    })
    Object.defineProperty(input.element, 'files', {
      value: [file]
    })

    await input.trigger('change')
    await wrapper.find('form').trigger('submit')
    await waitForImport()

    const cancelButton = wrapper
      .findAll('button')
      .find((button) => button.text() === 'common.cancel')
    await cancelButton?.trigger('click')

    expect(adminAPI.accounts.importData).not.toHaveBeenCalled()
    expect(wrapper.emitted('close')).toBeTruthy()
  })

  it('updates progress and errors per streamed inspected account', async () => {
    let releaseDone!: () => void
    const donePromise = new Promise<void>((resolve) => {
      releaseDone = resolve
    })
    vi.mocked(adminAPI.accounts.inspectDataStream).mockImplementation(async (_payload, onEvent) => {
      onEvent({
        type: 'item',
        item: {
          index: 0,
          name: 'bad-now',
          platform: 'openai',
          type: 'apikey',
          healthy: false,
          reasons: ['live probe failed: bad key']
        }
      })
      await donePromise
      onEvent({
        type: 'item',
        item: { index: 1, name: 'healthy-later', platform: 'openai', type: 'apikey', healthy: true }
      })
      onEvent({
        type: 'done',
        result: {
          total: 2,
          healthy: 1,
          unhealthy: 1,
          valid_proxy_keys: [],
          results: [
            { index: 0, name: 'bad-now', platform: 'openai', type: 'apikey', healthy: false, reasons: ['live probe failed: bad key'] },
            { index: 1, name: 'healthy-later', platform: 'openai', type: 'apikey', healthy: true }
          ]
        }
      })
    })
    vi.mocked(adminAPI.accounts.importData).mockResolvedValue({
      proxy_created: 0,
      proxy_reused: 0,
      proxy_failed: 0,
      account_created: 1,
      account_failed: 0
    })

    const wrapper = mount(ImportDataModal, {
      props: { show: true },
      global: {
        stubs: {
          BaseDialog: { template: '<div><slot /><slot name="footer" /></div>' }
        }
      }
    })

    const payload = {
      exported_at: '2026-01-01T00:00:00Z',
      proxies: [],
      accounts: [
        {
          name: 'bad-now',
          platform: 'openai',
          type: 'apikey',
          credentials: { email: 'bad-now@example.com', api_key: 'sk-bad' },
          concurrency: 1,
          priority: 50
        },
        {
          name: 'healthy-later',
          platform: 'openai',
          type: 'apikey',
          credentials: { api_key: 'sk-ok' },
          concurrency: 1,
          priority: 50
        }
      ]
    }

    const input = wrapper.find('input[type="file"]')
    const file = new File([JSON.stringify(payload)], 'stream.json', { type: 'application/json' })
    Object.defineProperty(file, 'text', {
      value: () => Promise.resolve(JSON.stringify(payload))
    })
    Object.defineProperty(input.element, 'files', {
      value: [file]
    })

    await input.trigger('change')
    await wrapper.find('form').trigger('submit')
    await flushPromises()
    await new Promise((resolve) => setTimeout(resolve, 0))

    expect(wrapper.text()).toContain('bad-now@example.com')
    expect(wrapper.text()).toContain('stream.json')
    expect(wrapper.text()).toContain('API Key')
    expect(wrapper.text()).toContain('admin.accounts.dataImportInspectStatusError')
    expect(wrapper.text()).toContain('live probe failed: bad key')
    expect(wrapper.text()).toContain('admin.accounts.dataImportProgressScanned')
    expect(adminAPI.accounts.importData).not.toHaveBeenCalled()
    expect(scrollTo).toHaveBeenCalled()

    releaseDone()
    await waitForImport()

    expect(adminAPI.accounts.importData).not.toHaveBeenCalled()
    expect(wrapper.text()).toContain('healthy-later')
    expect(wrapper.text()).toContain('admin.accounts.dataImportInspectStatusNormal')

    await wrapper.find('form').trigger('submit')
    await waitForImport()

    expect(adminAPI.accounts.importData).toHaveBeenCalledTimes(1)
  })

  it('uses final inspection result to reconcile progress counts', async () => {
    vi.mocked(adminAPI.accounts.inspectDataStream).mockImplementation(async (_payload, onEvent) => {
      onEvent({
        type: 'item',
        item: { index: 0, name: 'oauth-account', platform: 'openai', type: 'oauth', healthy: true }
      })
      onEvent({
        type: 'done',
        result: {
          total: 1,
          healthy: 0,
          unhealthy: 1,
          valid_proxy_keys: [],
          results: [
            {
              index: 0,
              name: 'oauth-account',
              platform: 'openai',
              type: 'oauth',
              healthy: false,
              reasons: ['live probe failed: unsupported model']
            }
          ]
        }
      })
    })

    const wrapper = mount(ImportDataModal, {
      props: { show: true },
      global: {
        stubs: {
          BaseDialog: { template: '<div><slot /><slot name="footer" /></div>' }
        }
      }
    })

    const payload = {
      exported_at: '2026-01-01T00:00:00Z',
      proxies: [],
      accounts: [
        {
          name: 'oauth-account',
          platform: 'openai',
          type: 'oauth',
          credentials: { email: 'oauth@example.com', access_token: 'token' },
          concurrency: 1,
          priority: 50
        }
      ]
    }

    const input = wrapper.find('input[type="file"]')
    const file = new File([JSON.stringify(payload)], 'oauth.json', { type: 'application/json' })
    Object.defineProperty(file, 'text', {
      value: () => Promise.resolve(JSON.stringify(payload))
    })
    Object.defineProperty(input.element, 'files', {
      value: [file]
    })

    await input.trigger('change')
    await wrapper.find('form').trigger('submit')
    await waitForImport()

    expect(wrapper.text()).toContain('admin.accounts.dataImportProgressScanned:1')
    expect(wrapper.text()).toContain('admin.accounts.dataImportProgressHealthy:0')
    expect(wrapper.text()).toContain('admin.accounts.dataImportProgressAbnormal:1')
    expect(adminAPI.accounts.importData).not.toHaveBeenCalled()
  })
})
