import { describe, it, expect, vi, beforeEach } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import ImportDataModal from '@/components/admin/account/ImportDataModal.vue'

const showError = vi.fn()
const showSuccess = vi.fn()
const showWarning = vi.fn()

vi.mock('@/stores/app', () => ({
  useAppStore: () => ({
    showError,
    showSuccess,
    showWarning
  })
}))

vi.mock('@/api/admin', () => ({
  adminAPI: {
    accounts: {
      importData: vi.fn()
    }
  }
}))

vi.mock('vue-i18n', () => ({
  useI18n: () => ({
    t: (key: string) => key
  })
}))

const mountModal = () =>
  mount(ImportDataModal, {
    props: { show: true },
    global: {
      stubs: {
        BaseDialog: { template: '<div><slot /><slot name="footer" /></div>' }
      }
    }
  })

const makeJsonFile = (name: string, content: string, type = 'application/json') => {
  const file = new File([content], name, { type })
  Object.defineProperty(file, 'text', {
    value: () => Promise.resolve(content)
  })
  return file
}

const setInputFiles = (element: Element, files: File[]) => {
  Object.defineProperty(element, 'files', {
    value: files,
    configurable: true
  })
}

describe('ImportDataModal', () => {
  beforeEach(async () => {
    showError.mockReset()
    showSuccess.mockReset()
    showWarning.mockReset()
    const { adminAPI } = await import('@/api/admin')
    vi.mocked(adminAPI.accounts.importData).mockReset()
  })

  it('未选择文件时提示错误', async () => {
    const wrapper = mountModal()

    await wrapper.find('form').trigger('submit')
    expect(showError).toHaveBeenCalledWith('admin.accounts.dataImportSelectFile')
  })

  it('无效 JSON 时按文件名提示解析失败', async () => {
    const { adminAPI } = await import('@/api/admin')
    const wrapper = mountModal()

    const input = wrapper.find('input[type="file"]')
    setInputFiles(input.element, [makeJsonFile('data.json', 'invalid json')])

    await input.trigger('change')
    await wrapper.find('form').trigger('submit')
    await flushPromises()

    expect(showError).toHaveBeenCalledWith('admin.accounts.dataImportParseFailedFile')
    expect(adminAPI.accounts.importData).not.toHaveBeenCalled()
  })

  it('不是导出数据的 JSON 按文件名拒绝', async () => {
    const { adminAPI } = await import('@/api/admin')
    const wrapper = mountModal()

    const input = wrapper.find('input[type="file"]')
    setInputFiles(input.element, [makeJsonFile('random.json', JSON.stringify({ name: 'test' }))])

    await input.trigger('change')
    await wrapper.find('form').trigger('submit')
    await flushPromises()

    expect(showError).toHaveBeenCalledWith('admin.accounts.dataImportInvalidFile')
    expect(adminAPI.accounts.importData).not.toHaveBeenCalled()
  })

  it('无有效 JSON 的选择不清空已有选择', async () => {
    const { adminAPI } = await import('@/api/admin')
    vi.mocked(adminAPI.accounts.importData).mockResolvedValue({
      proxy_created: 0,
      proxy_reused: 0,
      proxy_failed: 0,
      account_created: 1,
      account_failed: 0
    })

    const wrapper = mountModal()
    const input = wrapper.find('input[type="file"]')

    const valid = makeJsonFile(
      'valid.json',
      JSON.stringify({ exported_at: '2026-07-05T00:00:00Z', proxies: [], accounts: [{ name: 'a' }] })
    )
    setInputFiles(input.element, [valid])
    await input.trigger('change')

    setInputFiles(input.element, [new File(['hello'], 'notes.txt', { type: 'text/plain' })])
    await input.trigger('change')
    expect(showError).toHaveBeenCalledWith('admin.accounts.dataImportSelectFile')

    await wrapper.find('form').trigger('submit')
    await flushPromises()

    expect(adminAPI.accounts.importData).toHaveBeenCalledWith({
      data: expect.objectContaining({
        accounts: [{ name: 'a' }]
      }),
      skip_default_group_bind: true
    })
  })

  it('merges multiple selected JSON files before importing', async () => {
    const { adminAPI } = await import('@/api/admin')
    vi.mocked(adminAPI.accounts.importData).mockResolvedValue({
      proxy_created: 0,
      proxy_reused: 0,
      proxy_failed: 0,
      account_created: 2,
      account_failed: 0
    })

    const wrapper = mountModal()

    const input = wrapper.find('input[type="file"]')
    const first = makeJsonFile(
      'first.json',
      JSON.stringify({ exported_at: '2026-07-05T00:00:00Z', proxies: [], accounts: [{ name: 'a' }] })
    )
    const second = makeJsonFile(
      'second.json',
      JSON.stringify({
        exported_at: '2026-07-05T00:00:01Z',
        proxies: [{ proxy_key: 'p' }],
        accounts: [{ name: 'b' }]
      })
    )
    setInputFiles(input.element, [first, second])

    await input.trigger('change')
    await wrapper.find('form').trigger('submit')
    await flushPromises()

    expect(adminAPI.accounts.importData).toHaveBeenCalledWith({
      data: expect.objectContaining({
        proxies: [{ proxy_key: 'p' }],
        accounts: [{ name: 'a' }, { name: 'b' }]
      }),
      skip_default_group_bind: true
    })
    expect(showSuccess).toHaveBeenCalledWith('admin.accounts.dataImportSuccess')
  })

  it('preserves stable Codex profile proxy-key references when merging files', async () => {
    const { adminAPI } = await import('@/api/admin')
    vi.mocked(adminAPI.accounts.importData).mockResolvedValue({
      proxy_created: 0,
      proxy_reused: 0,
      proxy_failed: 0,
      account_created: 2,
      account_failed: 0
    })
    const wrapper = mountModal()
    const input = wrapper.find('input[type="file"]')
    const stableRefs = {
      linux: {
        proxy_key: 'proxy-linux',
        slot_proxy_keys: { '1': 'proxy-linux-slot-1' }
      }
    }
    setInputFiles(input.element, [
      makeJsonFile('first.json', JSON.stringify({
        exported_at: '2026-08-24T00:00:00Z',
        proxies: [],
        accounts: [{ name: 'oauth', codex_profile_proxies: stableRefs }]
      })),
      makeJsonFile('second.json', JSON.stringify({
        exported_at: '2026-08-24T00:00:01Z',
        proxies: [],
        accounts: [{ name: 'other' }]
      }))
    ])

    await input.trigger('change')
    await wrapper.find('form').trigger('submit')
    await flushPromises()

    expect(vi.mocked(adminAPI.accounts.importData).mock.calls[0]?.[0]?.data.accounts[0])
      .toMatchObject({ codex_profile_proxies: stableRefs })
  })

  it('部分成功时关闭弹窗仍通知父组件刷新', async () => {
    const { adminAPI } = await import('@/api/admin')
    vi.mocked(adminAPI.accounts.importData).mockResolvedValue({
      proxy_created: 0,
      proxy_reused: 0,
      proxy_failed: 0,
      account_created: 1,
      account_failed: 1
    })

    const wrapper = mountModal()
    const input = wrapper.find('input[type="file"]')
    setInputFiles(input.element, [
      makeJsonFile(
        'mixed.json',
        JSON.stringify({
          exported_at: '2026-07-05T00:00:00Z',
          proxies: [],
          accounts: [{ name: 'a' }, { name: 'b' }]
        })
      )
    ])

    await input.trigger('change')
    await wrapper.find('form').trigger('submit')
    await flushPromises()

    expect(showError).toHaveBeenCalledWith('admin.accounts.dataImportCompletedWithErrors')
    expect(wrapper.emitted('imported')).toBeUndefined()

    // 第二个 btn-secondary 是 footer 的取消按钮(第一个是选择文件)
    await wrapper.findAll('button.btn-secondary')[1]!.trigger('click')

    expect(wrapper.emitted('imported')).toHaveLength(1)
    expect(wrapper.emitted('close')).toHaveLength(1)
  })

  it('preserves imported identity policies by default for server-side proxy-key validation', async () => {
    const { adminAPI } = await import('@/api/admin')
    vi.mocked(adminAPI.accounts.importData).mockResolvedValue({
      proxy_created: 0,
      proxy_reused: 0,
      proxy_failed: 0,
      account_created: 1,
      account_failed: 0
    })
    const wrapper = mountModal()
    const input = wrapper.find('input[type="file"]')
    setInputFiles(input.element, [
      makeJsonFile('openai.json', JSON.stringify({
        exported_at: '2026-08-24T00:00:00Z',
        proxies: [],
        accounts: [{
          name: 'oauth',
          platform: 'openai',
          type: 'oauth',
          codex_identity_policy: {
            mode: 'os_profile_device_pool',
            version: 9,
            profiles: [{ os_class: 'linux', canonical_surface: 'cli', architecture: 'x86_64', slot_count: 1, proxy_id: 999, epoch: 4 }]
          }
        }]
      }))
    ])

    await input.trigger('change')
    await wrapper.find('form').trigger('submit')
    await flushPromises()

    const payload = vi.mocked(adminAPI.accounts.importData).mock.calls[0]?.[0]
    expect(payload?.data.accounts[0]?.codex_identity_policy).toMatchObject({
      mode: 'os_profile_device_pool',
      version: 9
    })
    expect(payload?.codex_identity_policy_override).toBeUndefined()
    expect(payload?.override_imported_identity_policies).toBeUndefined()
  })

  it('applies a current-deployment identity policy only after explicit opt-in', async () => {
    const { adminAPI } = await import('@/api/admin')
    vi.mocked(adminAPI.accounts.importData).mockResolvedValue({
      proxy_created: 0,
      proxy_reused: 0,
      proxy_failed: 0,
      account_created: 1,
      account_failed: 0
    })
    const wrapper = mountModal()
    const input = wrapper.find('input[type="file"]')
    setInputFiles(input.element, [
      makeJsonFile('openai.json', JSON.stringify({
        exported_at: '2026-08-24T00:00:00Z',
        proxies: [],
        accounts: [{ name: 'oauth', platform: 'openai', type: 'oauth' }]
      }))
    ])

    await input.trigger('change')
    await wrapper.get('[data-testid="import-codex-identity-override"]').setValue(true)
    await wrapper.find('form').trigger('submit')
    await flushPromises()

    const payload = vi.mocked(adminAPI.accounts.importData).mock.calls[0]?.[0]
    expect(payload?.codex_identity_policy_override).toEqual({
      mode: 'off',
      binding_scope: 'api_key_os_surface',
      session_policy: { mode: 'conversation_isolated' },
      affinity_ttl_seconds: 3600,
      unsupported_policy: 'reject',
      profiles: []
    })
    expect(payload?.override_imported_identity_policies).toBe(true)
  })
})
