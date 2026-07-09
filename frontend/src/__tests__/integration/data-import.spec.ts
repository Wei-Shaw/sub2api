import { describe, it, expect, vi, beforeEach } from 'vitest'
import { defineComponent, nextTick } from 'vue'
import { mount } from '@vue/test-utils'
import ImportDataModal from '@/components/admin/account/ImportDataModal.vue'
import { adminAPI } from '@/api/admin'

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
      importData: vi.fn()
    }
  }
}))

vi.mock('vue-i18n', async (importOriginal) => {
  const actual = await importOriginal<typeof import('vue-i18n')>()
  return {
    ...actual,
    useI18n: () => ({
      t: (key: string) => key
    })
  }
})

const BaseDialogStub = defineComponent({
  template: '<div><slot /><slot name="footer" /></div>'
})

const ModelWhitelistSelectorStub = defineComponent({
  props: {
    modelValue: {
      type: Array,
      default: () => []
    }
  },
  emits: ['update:modelValue'],
  template: '<button type="button" data-testid="set-models" @click="$emit(\'update:modelValue\', [\'gpt-image-2\'])">models</button>'
})

const mountModal = () =>
  mount(ImportDataModal, {
    props: {
      show: true,
      batches: [{ id: 7, name: 'batch' }]
    },
    global: {
      stubs: {
        BaseDialog: BaseDialogStub,
        ConfirmDialog: true,
        GroupSelector: true,
        ProxySelector: true,
        Icon: true,
        ModelWhitelistSelector: ModelWhitelistSelectorStub
      }
    }
  })

const setFile = async (wrapper: ReturnType<typeof mountModal>, content: string) => {
  const input = wrapper.find('input[type="file"]')
  const file = new File([content], 'data.json', { type: 'application/json' })
  Object.defineProperty(file, 'text', {
    value: () => Promise.resolve(content)
  })
  Object.defineProperty(input.element, 'files', {
    value: [file]
  })
  await input.trigger('change')
  await nextTick()
}

describe('ImportDataModal', () => {
  beforeEach(() => {
    showError.mockReset()
    showSuccess.mockReset()
    vi.mocked(adminAPI.accounts.importData).mockReset()
  })

  it('未选择文件时不提交导入', async () => {
    const wrapper = mountModal()

    await wrapper.get('[data-testid="confirm-import"]').trigger('click')

    expect(adminAPI.accounts.importData).not.toHaveBeenCalled()
    expect(showError).not.toHaveBeenCalled()
  })

  it('无效 JSON 时提示解析失败', async () => {
    const wrapper = mountModal()
    await setFile(wrapper, 'invalid json')
    await wrapper.find('select').setValue('7')

    await wrapper.get('[data-testid="confirm-import"]').trigger('click')
    await nextTick()

    expect(showError).toHaveBeenCalledWith('admin.accounts.dataImportParseFailed')
  })

  it('导入时提交模型白名单和自动检测配置', async () => {
    vi.mocked(adminAPI.accounts.importData).mockResolvedValue({
      proxy_created: 0,
      proxy_reused: 0,
      proxy_failed: 0,
      account_created: 1,
      account_failed: 0,
      model_sync_succeeded: 1,
      model_sync_failed: 0
    })
    const wrapper = mountModal()
    const content = JSON.stringify({
      type: 'sub2api-data',
      version: 1,
      proxies: [],
      accounts: [
        {
          name: 'openai-key',
          platform: 'openai',
          type: 'apikey',
          credentials: { api_key: 'sk-test' },
          concurrency: 3,
          priority: 50
        }
      ]
    })

    await setFile(wrapper, content)
    await wrapper.find('select').setValue('7')
    await wrapper.get('[data-testid="set-models"]').trigger('click')
    await wrapper.get('[data-testid="auto-detect-models-toggle"]').trigger('click')
    await wrapper.get('[data-testid="confirm-import"]').trigger('click')
    await nextTick()

    expect(adminAPI.accounts.importData).toHaveBeenCalledTimes(1)
    expect(adminAPI.accounts.importData).toHaveBeenCalledWith(
      expect.objectContaining({
        batch_id: 7,
        concurrency: 1,
        schedulable: false,
        auto_detect_models: true,
        credential_extras: {
          model_mapping: {
            'gpt-image-2': 'gpt-image-2'
          }
        }
      })
    )
  })

  it('允许提交 cockpit-tools data-transfer 格式', async () => {
    vi.mocked(adminAPI.accounts.importData).mockResolvedValue({
      proxy_created: 0,
      proxy_reused: 0,
      proxy_failed: 0,
      account_created: 1,
      account_failed: 0
    })
    const wrapper = mountModal()
    const content = JSON.stringify({
      schema: 'cockpit-tools.data-transfer',
      version: 1,
      exported_at: '2026-05-23T00:00:00Z',
      sections: {
        accounts: true,
        config: false
      },
      accounts: {
        schema: 'cockpit-tools.account-transfer',
        version: 1,
        exported_at: '2026-05-23T00:00:00Z',
        platforms: {
          codex: {
            account_count: 1,
            exported_data: [
              {
                email: 'codex@example.com',
                auth_mode: 'apikey',
                openai_api_key: 'sk-test'
              }
            ]
          }
        }
      }
    })

    await setFile(wrapper, content)
    await wrapper.find('select').setValue('7')
    await wrapper.get('[data-testid="confirm-import"]').trigger('click')
    await nextTick()

    expect(adminAPI.accounts.importData).toHaveBeenCalledTimes(1)
    expect(adminAPI.accounts.importData).toHaveBeenCalledWith(
      expect.objectContaining({
        batch_id: 7,
        data: expect.objectContaining({
          schema: 'cockpit-tools.data-transfer'
        })
      })
    )
  })
})
