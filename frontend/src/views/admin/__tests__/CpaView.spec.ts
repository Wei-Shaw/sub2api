import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'

import CpaView from '../CpaView.vue'

/**
 * 组件级回归测试：直接复现「打开 CPA 面板凭据页」的路径。
 * 曾经的 bug：api 层多解了一层导致 listAuthFiles() 返回 undefined，
 * 页面报 `Cannot read properties of undefined (reading 'files')`。
 */
const {
  getConfig,
  updateConfig,
  testConnection,
  getOverview,
  listAuthFiles,
  getAuthFileQuota,
  getApiKeyUsage,
  getLogs,
  listErrorLogs,
  getErrorLogContent,
  showError,
  showSuccess,
} = vi.hoisted(() => ({
  getConfig: vi.fn(),
  updateConfig: vi.fn(),
  testConnection: vi.fn(),
  getOverview: vi.fn(),
  listAuthFiles: vi.fn(),
  getAuthFileQuota: vi.fn(),
  getApiKeyUsage: vi.fn(),
  getLogs: vi.fn(),
  listErrorLogs: vi.fn(),
  getErrorLogContent: vi.fn(),
  showError: vi.fn(),
  showSuccess: vi.fn(),
}))

vi.mock('@/api/cpa', () => ({
  cpaApi: {
    getConfig,
    updateConfig,
    testConnection,
    getOverview,
    listAuthFiles,
    getAuthFileQuota,
    getApiKeyUsage,
    getLogs,
    listErrorLogs,
    getErrorLogContent,
  },
}))

vi.mock('@/stores', () => ({
  useAppStore: () => ({ showError, showSuccess }),
}))

// 布局组件会牵出 router / 侧边栏 / 更多 store，这里只保留插槽。
vi.mock('@/components/layout/AppLayout.vue', () => ({
  default: { name: 'AppLayout', template: '<div><slot /></div>' },
}))

// vitest 里 vue-i18n 是 runtime-only 构建（无消息编译器），按项目惯例替换 t()，
// 文案直接取真实的 en locale，保证断言的是真文案。
vi.mock('vue-i18n', async (importOriginal) => {
  const actual = await importOriginal<typeof import('vue-i18n')>()
  const { default: en } = await import('@/i18n/locales/en')

  const lookup = (path: string): unknown =>
    path.split('.').reduce<unknown>((acc, part) => {
      if (acc && typeof acc === 'object') return (acc as Record<string, unknown>)[part]
      return undefined
    }, en)

  return {
    ...actual,
    useI18n: () => ({
      t: (key: string, params?: Record<string, unknown>) => {
        const found = lookup(key)
        let value = typeof found === 'string' ? found : key
        if (params) {
          for (const [name, replacement] of Object.entries(params)) {
            value = value.replace(new RegExp(`\\{${name}\\}`, 'g'), String(replacement))
          }
        }
        return value
      },
      locale: { value: 'en' },
    }),
  }
})

async function mountView() {
  const wrapper = mount(CpaView)
  await flushPromises()
  await flushPromises()
  return wrapper
}

const AUTH_FILE = {
  id: 'mock-codex-auth',
  auth_index: 'idx-1',
  name: 'codex-mock-birgit-pro.json',
  provider: 'codex',
  status: 'active',
  email: 'mock@example.com',
  success: 724,
  failed: 1,
  recent_requests: [
    { time: '2026-09-09T10:00:00Z', success: 3, failed: 0 },
    { time: '2026-09-09T10:05:00Z', success: 0, failed: 1 },
  ],
  quota: {
    observed_at: '2026-09-09T10:00:00Z',
    signals: { 'X-Codex-Primary-Used-Percent': '14' },
  },
}

const CODEX_USAGE = {
  plan_type: 'pro',
  rate_limit: {
    primary_window: { used_percent: 14, limit_window_seconds: 604800, reset_after_seconds: 516778 },
  },
  rate_limit_reset_credits: { available_count: 1, applicable_available_count: 0 },
}

beforeEach(() => {
  vi.clearAllMocks()
  getConfig.mockResolvedValue({
    enabled: true,
    base_url: 'http://host.docker.internal:8317',
    management_key_configured: true,
    management_key_hint: 'lI******34',
  })
  listAuthFiles.mockResolvedValue({ files: [AUTH_FILE] })
  getAuthFileQuota.mockResolvedValue(CODEX_USAGE)
  getApiKeyUsage.mockResolvedValue({})
  getLogs.mockRejectedValue(new Error('logging to file disabled'))
  listErrorLogs.mockResolvedValue({ files: [] })
  getErrorLogContent.mockResolvedValue({ name: 'x.log', content: 'line' })
})

describe('CpaView', () => {
  it('renders the credential list without the undefined "files" crash', async () => {
    const wrapper = await mountView()

    expect(wrapper.text()).toContain('codex-mock-birgit-pro.json')
    expect(wrapper.text()).toContain('mock@example.com')
    expect(wrapper.text()).not.toContain('Failed to load')
    expect(wrapper.text()).not.toContain('reading')
  })

  it('loads the real Codex quota for codex credentials', async () => {
    const wrapper = await mountView()

    expect(getAuthFileQuota).toHaveBeenCalledWith('idx-1')
    // plan_type 来自真实额度响应
    expect(wrapper.text()).toContain('pro')
    // 604800s 窗口应被识别为周窗口（而非硬编码的 5h）
    expect(wrapper.text()).toContain('Weekly window')
    expect(wrapper.text()).not.toContain('5h window')
  })

  it('shows success/failed counters and passive quota signals', async () => {
    const wrapper = await mountView()

    expect(wrapper.text()).toContain('724')
    expect(wrapper.text()).toContain('X-Codex-Primary-Used-Percent')
    expect(wrapper.text()).toContain('14')
  })

  it('renders the redesigned card sections (stats, quota title, success rate)', async () => {
    const wrapper = await mountView()

    expect(wrapper.text()).toContain('Quota')
    // 724 / (724 + 1) = 99.9%
    expect(wrapper.text()).toContain('99.9%')
    // 顶部状态色条按状态着色
    expect(wrapper.find('.bg-emerald-400').exists()).toBe(true)
    // 供应商图标容器
    expect(wrapper.find('.bg-emerald-50').exists()).toBe(true)
  })

  it('shows a placeholder before the quota is fetched', async () => {
    // 额度请求悬挂：既没有数据也没有错误，应显示占位提示
    getAuthFileQuota.mockReturnValue(new Promise(() => {}) as never)

    const wrapper = await mountView()

    expect(wrapper.text()).toContain('Not fetched yet')
  })

  it('surfaces a load failure instead of crashing', async () => {
    listAuthFiles.mockRejectedValueOnce(new Error('CPA is unreachable'))

    const wrapper = await mountView()

    expect(wrapper.text()).toContain('CPA is unreachable')
    expect(wrapper.text()).toContain('Failed to load')
  })
})
