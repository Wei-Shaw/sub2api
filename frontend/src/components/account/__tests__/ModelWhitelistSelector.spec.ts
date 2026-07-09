import { describe, expect, it, vi, beforeEach } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import ModelWhitelistSelector from '../ModelWhitelistSelector.vue'
import { accountsAPI } from '@/api/admin/accounts'

const showError = vi.fn()

vi.mock('@/stores/app', () => ({
  useAppStore: () => ({
    showError,
    showSuccess: vi.fn(),
    showInfo: vi.fn()
  })
}))

vi.mock('@/api/admin/accounts', () => ({
  accountsAPI: {
    syncUpstreamModels: vi.fn(),
    syncUpstreamModelsPreview: vi.fn()
  }
}))

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return {
    ...actual,
    useI18n: () => ({
      t: (key: string, params?: Record<string, unknown>) => {
        if (key === 'admin.accounts.syncUpstreamModels') return '同步上游支持的模型'
        if (key === 'admin.accounts.syncUpstreamModelsLoading') return '同步上游中...'
        if (key === 'admin.accounts.syncUpstreamModelsError') return `同步上游模型失败：${params?.message}`
        if (key === 'admin.accounts.syncUpstreamModelsFailed') return '同步上游模型失败'
        return key
      }
    })
  }
})

describe('ModelWhitelistSelector', () => {
  beforeEach(() => {
    vi.mocked(accountsAPI.syncUpstreamModels).mockReset()
    vi.mocked(accountsAPI.syncUpstreamModelsPreview).mockReset()
    showError.mockReset()
  })

  it('同步上游模型失败时显示后端返回的具体原因', async () => {
    vi.mocked(accountsAPI.syncUpstreamModels).mockRejectedValue({
      response: {
        data: {
          message: 'Upstream model list request failed with HTTP 401'
        }
      }
    })

    const wrapper = mount(ModelWhitelistSelector, {
      props: {
        modelValue: [],
        platform: 'gemini',
        accountId: 1
      },
      global: {
        stubs: {
          ModelIcon: true,
          Icon: true
        }
      }
    })

    await wrapper.get('button:nth-of-type(2)').trigger('click')
    await flushPromises()

    expect(wrapper.text()).toContain('同步上游模型失败：Upstream model list request failed with HTTP 401')
    expect(showError).toHaveBeenCalledWith(
      '同步上游模型失败：Upstream model list request failed with HTTP 401',
      8000
    )
  })
})
