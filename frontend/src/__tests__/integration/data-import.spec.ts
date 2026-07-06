import { describe, it, expect, vi, beforeEach } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
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
    },
    groups: {
      list: vi.fn()
    }
  }
}))

vi.mock('vue-i18n', () => ({
  useI18n: () => ({
    t: (key: string, params?: Record<string, unknown>) =>
      params ? `${key} ${JSON.stringify(params)}` : key
  })
}))

describe('ImportDataModal', () => {
  beforeEach(() => {
    showError.mockReset()
    showSuccess.mockReset()
    vi.mocked(adminAPI.accounts.importData).mockReset()
    vi.mocked(adminAPI.groups.list).mockReset()
    vi.mocked(adminAPI.groups.list).mockResolvedValue({
      items: [],
      total: 0,
      page: 1,
      page_size: 1000,
      pages: 1
    })
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
    await Promise.resolve()

    expect(showError).toHaveBeenCalledWith('admin.accounts.dataImportParseFailed')
  })

  it('加载所有未删除分组并标识停用分组', async () => {
    vi.mocked(adminAPI.groups.list).mockResolvedValue({
      items: [
        createGroup({ id: 1, name: 'Claude 主分组', platform: 'anthropic', status: 'active' }),
        createGroup({ id: 2, name: 'Claude 停用分组', platform: 'anthropic', status: 'inactive' })
      ],
      total: 2,
      page: 1,
      page_size: 1000,
      pages: 1
    })

    const wrapper = mount(ImportDataModal, {
      props: { show: true },
      global: {
        stubs: {
          BaseDialog: { template: '<div><slot /><slot name="footer" /></div>' }
        }
      }
    })

    await Promise.resolve()

    expect(adminAPI.groups.list).toHaveBeenCalledWith(1, 1000, undefined)
    expect(wrapper.text()).toContain('Claude 主分组')
    expect(wrapper.text()).toContain('Claude 停用分组')
    expect(wrapper.text()).toContain('admin.accounts.dataImportGroupInactive')

    const inactiveCheckbox = wrapper
      .findAll('label')
      .find((label) => label.text().includes('Claude 停用分组'))
      ?.find('input[type="checkbox"]')
    expect(inactiveCheckbox?.exists()).toBe(true)
    expect(inactiveCheckbox?.attributes('disabled')).toBeUndefined()
  })

  it('选择导入目标分组后提交请求携带 group_ids 且不修改数据导入文件', async () => {
    vi.mocked(adminAPI.groups.list).mockResolvedValue({
      items: [
        createGroup({ id: 1, name: 'Claude 主分组', platform: 'anthropic' }),
        createGroup({ id: 2, name: 'Claude 停用分组', platform: 'anthropic', status: 'inactive' })
      ],
      total: 2,
      page: 1,
      page_size: 1000,
      pages: 1
    })
    vi.mocked(adminAPI.accounts.importData).mockResolvedValue({
      proxy_created: 0,
      proxy_reused: 0,
      proxy_failed: 0,
      account_created: 1,
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
    await flushPromises()

    const checkboxes = wrapper.findAll('input[type="checkbox"]')
    await checkboxes[0].setValue(true)
    await checkboxes[1].setValue(true)

    const dataPayload = {
      exported_at: '2026-01-01T00:00:00Z',
      proxies: [],
      accounts: [
        {
          name: 'Claude Account',
          platform: 'anthropic',
          type: 'oauth',
          credentials: {},
          concurrency: 1,
          priority: 1
        }
      ]
    }
    const input = wrapper.find('input[type="file"]')
    const file = new File([JSON.stringify(dataPayload)], 'data.json', { type: 'application/json' })
    Object.defineProperty(file, 'text', {
      value: () => Promise.resolve(JSON.stringify(dataPayload))
    })
    Object.defineProperty(input.element, 'files', {
      value: [file]
    })

    await input.trigger('change')
    await wrapper.find('form').trigger('submit')
    await flushPromises()

    expect(adminAPI.accounts.importData).toHaveBeenCalledWith({
      data: dataPayload,
      skip_default_group_bind: true,
      group_ids: [1, 2]
    })
    expect(dataPayload).not.toHaveProperty('group_ids')
  })

  it('提交前拦截所选导入目标分组跨平台', async () => {
    vi.mocked(adminAPI.groups.list).mockResolvedValue({
      items: [
        createGroup({ id: 1, name: 'Claude 分组', platform: 'anthropic' }),
        createGroup({ id: 3, name: 'OpenAI 分组', platform: 'openai' })
      ],
      total: 2,
      page: 1,
      page_size: 1000,
      pages: 1
    })

    const wrapper = mount(ImportDataModal, {
      props: { show: true },
      global: {
        stubs: {
          BaseDialog: { template: '<div><slot /><slot name="footer" /></div>' }
        }
      }
    })
    await flushPromises()

    const checkboxes = wrapper.findAll('input[type="checkbox"]')
    await checkboxes[0].setValue(true)
    await checkboxes[1].setValue(true)

    await setImportFile(wrapper, {
      exported_at: '2026-01-01T00:00:00Z',
      proxies: [],
      accounts: [
        {
          name: 'Claude Account',
          platform: 'anthropic',
          type: 'oauth',
          credentials: {},
          concurrency: 1,
          priority: 1
        }
      ]
    })
    await wrapper.find('form').trigger('submit')
    await flushPromises()

    expect(adminAPI.accounts.importData).not.toHaveBeenCalled()
    expect(showError).toHaveBeenCalledWith('admin.accounts.dataImportTargetGroupMixedPlatforms')
  })

  it('提交前拦截上游账号平台不匹配并限制示例数量', async () => {
    vi.mocked(adminAPI.groups.list).mockResolvedValue({
      items: [
        createGroup({ id: 1, name: 'Claude 分组', platform: 'anthropic' })
      ],
      total: 1,
      page: 1,
      page_size: 1000,
      pages: 1
    })

    const wrapper = mount(ImportDataModal, {
      props: { show: true },
      global: {
        stubs: {
          BaseDialog: { template: '<div><slot /><slot name="footer" /></div>' }
        }
      }
    })
    await flushPromises()

    await wrapper.find('input[type="checkbox"]').setValue(true)
    await setImportFile(wrapper, {
      exported_at: '2026-01-01T00:00:00Z',
      proxies: [],
      accounts: [
        {
          name: 'Claude Account',
          platform: 'anthropic',
          type: 'oauth',
          credentials: {},
          concurrency: 1,
          priority: 1
        },
        ...Array.from({ length: 6 }, (_, index) => ({
          name: `Mismatch ${index + 1}`,
          platform: index % 2 === 0 ? 'openai' : 'gemini',
          type: 'oauth',
          credentials: {},
          concurrency: 1,
          priority: 1
        }))
      ]
    })

    await wrapper.find('form').trigger('submit')
    await flushPromises()

    expect(adminAPI.accounts.importData).not.toHaveBeenCalled()
    const message = showError.mock.calls.at(-1)?.[0] ?? ''
    expect(message).toContain('admin.accounts.dataImportAccountPlatformMismatch')
    expect(message).toContain('"expected_platform":"anthropic"')
    expect(message).toContain('"mismatch_count":6')
    expect(message).toContain('Mismatch 1')
    expect(message).toContain('openai')
    expect(message).toContain('Mismatch 5')
    expect(message).not.toContain('Mismatch 6')
  })
})

const setImportFile = async (wrapper: ReturnType<typeof mount>, dataPayload: Record<string, unknown>) => {
  const input = wrapper.find('input[type="file"]')
  const file = new File([JSON.stringify(dataPayload)], 'data.json', { type: 'application/json' })
  Object.defineProperty(file, 'text', {
    value: () => Promise.resolve(JSON.stringify(dataPayload))
  })
  Object.defineProperty(input.element, 'files', {
    value: [file]
  })

  await input.trigger('change')
}

const createGroup = (overrides: Record<string, unknown>) => ({
  id: 1,
  name: 'Group',
  description: null,
  platform: 'anthropic',
  rate_multiplier: 1,
  is_exclusive: false,
  status: 'active',
  subscription_type: 'standard',
  daily_limit_usd: null,
  weekly_limit_usd: null,
  monthly_limit_usd: null,
  allow_image_generation: true,
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
  updated_at: '2026-01-01T00:00:00Z',
  model_routing: null,
  model_routing_enabled: false,
  mcp_xml_inject: false,
  account_count: 0,
  sort_order: 0,
  ...overrides
})
