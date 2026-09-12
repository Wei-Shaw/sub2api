import { describe, it, expect, vi, beforeEach } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import { adminAPI } from '@/api/admin'
import type { AdminDataImportResult } from '@/types'
import ImportDataModal from '@/components/admin/proxy/ImportDataModal.vue'

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
    proxies: {
      importData: vi.fn()
    }
  }
}))

vi.mock('vue-i18n', () => ({
  useI18n: () => ({
    t: (key: string) => key
  })
}))

describe('Proxy ImportDataModal', () => {
  const partialResult: AdminDataImportResult = {
    proxy_created: 1,
    proxy_reused: 0,
    proxy_failed: 1,
    account_created: 0,
    account_failed: 0,
    errors: [{ kind: 'proxy', name: 'invalid proxy', message: 'invalid port' }]
  }

  const mountWithFile = async () => {
    const wrapper = mount(ImportDataModal, {
      props: { show: true },
      global: {
        stubs: {
          BaseDialog: { template: '<div><slot /><footer><slot name="footer" /></footer></div>' }
        }
      }
    })
    const input = wrapper.find('input[type="file"]')
    const file = new File(['{}'], 'data.json', { type: 'application/json' })
    Object.defineProperty(file, 'text', { value: () => Promise.resolve('{}') })
    Object.defineProperty(input.element, 'files', { value: [file] })
    await input.trigger('change')
    return wrapper
  }

  it.each([
    { name: 'created', proxy_created: 1, proxy_reused: 0 },
    { name: 'reused', proxy_created: 0, proxy_reused: 1 }
  ])('refreshes $name proxies when closing a partial import result', async (counts) => {
    vi.mocked(adminAPI.proxies.importData).mockResolvedValue({ ...partialResult, ...counts })
    const wrapper = await mountWithFile()

    await wrapper.find('form').trigger('submit')
    await flushPromises()

    expect(showError).toHaveBeenCalledWith('admin.proxies.dataImportCompletedWithErrors')
    expect(wrapper.text()).toContain('invalid port')
    expect(wrapper.emitted('imported')).toBeUndefined()
    expect(wrapper.emitted('close')).toBeUndefined()

    await wrapper.get('footer button[type="button"]').trigger('click')
    expect(wrapper.emitted('imported')).toHaveLength(1)
    expect(wrapper.emitted('close')).toHaveLength(1)

    await wrapper.setProps({ show: false })
    await wrapper.setProps({ show: true })
    await wrapper.get('footer button[type="button"]').trigger('click')
    expect(wrapper.emitted('imported')).toHaveLength(1)
  })

  it('keeps a pending refresh when a later import entirely fails', async () => {
    vi.mocked(adminAPI.proxies.importData)
      .mockResolvedValueOnce(partialResult)
      .mockResolvedValueOnce({ ...partialResult, proxy_created: 0 })
    const wrapper = await mountWithFile()

    await wrapper.find('form').trigger('submit')
    await flushPromises()
    await wrapper.find('form').trigger('submit')
    await flushPromises()
    await wrapper.get('footer button[type="button"]').trigger('click')

    expect(wrapper.emitted('imported')).toHaveLength(1)
  })

  it('does not refresh when all imported proxies fail', async () => {
    vi.mocked(adminAPI.proxies.importData).mockResolvedValue({ ...partialResult, proxy_created: 0 })
    const wrapper = await mountWithFile()

    await wrapper.find('form').trigger('submit')
    await flushPromises()
    await wrapper.get('footer button[type="button"]').trigger('click')

    expect(wrapper.emitted('imported')).toBeUndefined()
    expect(wrapper.emitted('close')).toHaveLength(1)
  })

  it('refreshes immediately after a successful retry without refreshing twice on close', async () => {
    vi.mocked(adminAPI.proxies.importData)
      .mockResolvedValueOnce(partialResult)
      .mockResolvedValueOnce({ ...partialResult, proxy_failed: 0, errors: [] })
    const wrapper = await mountWithFile()

    await wrapper.find('form').trigger('submit')
    await flushPromises()
    await wrapper.find('form').trigger('submit')
    await flushPromises()
    expect(showSuccess).toHaveBeenCalledWith('admin.proxies.dataImportSuccess')
    expect(wrapper.emitted('imported')).toHaveLength(1)

    await wrapper.get('footer button[type="button"]').trigger('click')
    expect(wrapper.emitted('imported')).toHaveLength(1)
  })

  beforeEach(() => {
    showError.mockReset()
    showSuccess.mockReset()
    vi.mocked(adminAPI.proxies.importData).mockReset()
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
    expect(showError).toHaveBeenCalledWith('admin.proxies.dataImportSelectFile')
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
    await Promise.resolve()

    expect(showError).toHaveBeenCalledWith('admin.proxies.dataImportParseFailed')
  })
})
