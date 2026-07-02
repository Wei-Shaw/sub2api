import { describe, it, expect, vi, beforeEach } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import ImportDataModal from '@/components/admin/account/ImportDataModal.vue'
import { mergeAccountImportPayloads, normalizeAccountImportPayload } from '@/utils/adminDataImport'

const { importData } = vi.hoisted(() => ({
  importData: vi.fn()
}))

const showError = vi.fn()
const showSuccess = vi.fn()

vi.mock('@/stores/app', () => ({
  useAppStore: () => ({
    showError,
    showSuccess
  })
}))

vi.mock('@/api/admin', () => ({
  adminAPI: {
    accounts: {
      importData
    }
  }
}))

vi.mock('vue-i18n', () => ({
  useI18n: () => ({
    t: (key: string) => key
  })
}))

describe('ImportDataModal', () => {
  beforeEach(() => {
    showError.mockReset()
    showSuccess.mockReset()
    importData.mockReset()
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
    await flushPromises()

    expect(showError).toHaveBeenCalledWith('admin.accounts.dataImportParseFailed')
  })

  it('支持纯账号数组批量导入', async () => {
    importData.mockResolvedValue({
      proxy_created: 0,
      proxy_reused: 0,
      proxy_failed: 0,
      account_created: 2,
      account_failed: 0,
      errors: []
    })

    const wrapper = mount(ImportDataModal, {
      props: { show: true },
      global: {
        stubs: {
          BaseDialog: { template: '<div><slot /><slot name="footer" /></div>' }
        }
      }
    })

    const input = wrapper.find('input[type="file"]')
    const file = new File(
      [
        JSON.stringify([
          {
            name: 'acc-1',
            platform: 'openai',
            type: 'oauth',
            credentials: { access_token: 'token-1' },
            concurrency: 2,
            priority: 10
          },
          {
            name: 'acc-2',
            platform: 'gemini',
            type: 'apikey',
            credentials: { api_key: 'token-2' },
            concurrency: 3,
            priority: 20
          }
        ])
      ],
      'data.json',
      { type: 'application/json' }
    )
    Object.defineProperty(file, 'text', {
      value: () =>
        Promise.resolve(
          JSON.stringify([
            {
              name: 'acc-1',
              platform: 'openai',
              type: 'oauth',
              credentials: { access_token: 'token-1' },
              concurrency: 2,
              priority: 10
            },
            {
              name: 'acc-2',
              platform: 'gemini',
              type: 'apikey',
              credentials: { api_key: 'token-2' },
              concurrency: 3,
              priority: 20
            }
          ])
        )
    })
    Object.defineProperty(input.element, 'files', {
      value: [file]
    })

    await input.trigger('change')
    await wrapper.find('form').trigger('submit')
    await flushPromises()

    expect(importData).toHaveBeenCalledTimes(1)
    expect(importData).toHaveBeenCalledWith({
      data: {
        type: 'sub2api-data',
        version: 1,
        exported_at: expect.any(String),
        proxies: [],
        accounts: [
          {
            name: 'acc-1',
            notes: null,
            platform: 'openai',
            type: 'oauth',
            credentials: { access_token: 'token-1' },
            extra: undefined,
            proxy_key: null,
            concurrency: 2,
            priority: 10,
            rate_multiplier: null,
            expires_at: null,
            auto_pause_on_expired: undefined
          },
          {
            name: 'acc-2',
            notes: null,
            platform: 'gemini',
            type: 'apikey',
            credentials: { api_key: 'token-2' },
            extra: undefined,
            proxy_key: null,
            concurrency: 3,
            priority: 20,
            rate_multiplier: null,
            expires_at: null,
            auto_pause_on_expired: undefined
          }
        ]
      },
      skip_default_group_bind: true
    })
    expect(showSuccess).toHaveBeenCalledWith('admin.accounts.dataImportSuccess')
  })

  it('支持一次选择多个单独 json 文件', async () => {
    importData.mockResolvedValue({
      proxy_created: 0,
      proxy_reused: 0,
      proxy_failed: 0,
      account_created: 2,
      account_failed: 0,
      errors: []
    })

    const wrapper = mount(ImportDataModal, {
      props: { show: true },
      global: {
        stubs: {
          BaseDialog: { template: '<div><slot /><slot name="footer" /></div>' }
        }
      }
    })

    const payloadA = {
      name: 'acc-a',
      platform: 'openai',
      type: 'oauth',
      credentials: { access_token: 'token-a' },
      concurrency: 1,
      priority: 10
    }
    const payloadB = {
      name: 'acc-b',
      platform: 'gemini',
      type: 'apikey',
      credentials: { api_key: 'token-b' },
      concurrency: 2,
      priority: 20
    }

    const fileA = new File([JSON.stringify(payloadA)], 'a.json', { type: 'application/json' })
    const fileB = new File([JSON.stringify(payloadB)], 'b.json', { type: 'application/json' })
    Object.defineProperty(fileA, 'text', { value: () => Promise.resolve(JSON.stringify(payloadA)) })
    Object.defineProperty(fileB, 'text', { value: () => Promise.resolve(JSON.stringify(payloadB)) })

    const input = wrapper.find('input[type="file"]')
    Object.defineProperty(input.element, 'files', {
      value: [fileA, fileB]
    })

    await input.trigger('change')
    await wrapper.find('form').trigger('submit')
    await flushPromises()

    const expectedPayload = mergeAccountImportPayloads([
      normalizeAccountImportPayload(payloadA),
      normalizeAccountImportPayload(payloadB)
    ])

    expect(importData).toHaveBeenCalledWith({
      data: {
        ...expectedPayload,
        exported_at: expect.any(String)
      },
      skip_default_group_bind: true
    })
  })
})
