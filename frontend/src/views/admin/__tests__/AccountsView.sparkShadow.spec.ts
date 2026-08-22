import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'

import AccountsView from '../AccountsView.vue'
import AccountActionMenu from '@/components/admin/account/AccountActionMenu.vue'
import PlatformTypeBadge from '@/components/common/PlatformTypeBadge.vue'
import ConfirmDialog from '@/components/common/ConfirmDialog.vue'
import HelpTooltip from '@/components/common/HelpTooltip.vue'

// 外审 F2:AccountActionMenu emit 'create-spark-shadow',但 AccountsView 此前未监听,
// 导致按钮点击无效。本测试通过真实组件引用 emit 该事件,断言父页面接线调用 API。
const {
  listAccounts,
  listWithEtag,
  getBatchTodayStats,
  getAllProxies,
  getAllGroups,
  duplicateAccount,
  createSparkShadow,
  showSuccess,
  showError
} = vi.hoisted(() => ({
  listAccounts: vi.fn(),
  listWithEtag: vi.fn(),
  getBatchTodayStats: vi.fn(),
  getAllProxies: vi.fn(),
  getAllGroups: vi.fn(),
  duplicateAccount: vi.fn(),
  createSparkShadow: vi.fn(),
  showSuccess: vi.fn(),
  showError: vi.fn()
}))

vi.mock('@/api/admin', () => ({
  adminAPI: {
    accounts: {
      list: listAccounts,
      listWithEtag,
      getBatchTodayStats,
      duplicate: duplicateAccount,
      getUpstreamBillingProbeSettings: vi.fn().mockResolvedValue({ enabled: true, interval_minutes: 30 }),
      createSparkShadow,
      delete: vi.fn(),
      batchClearError: vi.fn(),
      batchRefresh: vi.fn(),
      toggleSchedulable: vi.fn()
    },
    proxies: { getAll: getAllProxies },
    groups: { getAll: getAllGroups }
  }
}))

vi.mock('@/stores/app', () => ({
  useAppStore: () => ({ showError, showSuccess, showInfo: vi.fn() })
}))

vi.mock('@/stores/auth', () => ({
  useAuthStore: () => ({ token: 'test-token' })
}))

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return {
    ...actual,
    useI18n: () => ({ t: (key: string) => key })
  }
})

const mountView = () =>
  mount(AccountsView, {
    global: {
      stubs: {
        AppLayout: { template: '<div><slot /></div>' },
        TablePageLayout: {
          template: '<div><slot name="filters" /><slot name="table" /><slot name="pagination" /></div>'
        },
        DataTable: true,
        Pagination: true,
        ConfirmDialog: true,
        AccountTableActions: { template: '<div><slot name="beforeCreate" /><slot name="after" /></div>' },
        AccountTableFilters: { template: '<div></div>' },
        AccountBulkActionsBar: true,
        AccountActionMenu: true,
        ImportDataModal: true,
        ReAuthAccountModal: true,
        AccountTestModal: true,
        AccountStatsModal: true,
        ScheduledTestsPanel: true,
        SyncFromCrsModal: true,
        TempUnschedStatusModal: true,
        ErrorPassthroughRulesModal: true,
        TLSFingerprintProfilesModal: true,
        CreateAccountModal: true,
        EditAccountModal: true,
        BulkEditAccountModal: true,
        PlatformTypeBadge: true,
        AccountCapacityCell: true,
        AccountStatusIndicator: true,
        AccountTodayStatsCell: true,
        AccountGroupsCell: true,
        AccountUsageCell: true,
        Icon: true
      }
    }
  })

describe('admin AccountsView — 外审 F2:spark 影子创建接线', () => {
  beforeEach(() => {
    localStorage.clear()
    for (const fn of [listAccounts, listWithEtag, getBatchTodayStats, getAllProxies, getAllGroups, duplicateAccount, createSparkShadow, showSuccess, showError]) {
      fn.mockReset()
    }
    listAccounts.mockResolvedValue({ items: [], total: 0, page: 1, page_size: 20, pages: 0 })
    listWithEtag.mockResolvedValue({ notModified: true, etag: null, data: null })
    getBatchTodayStats.mockResolvedValue({ stats: {} })
    getAllProxies.mockResolvedValue([])
    getAllGroups.mockResolvedValue([])
    duplicateAccount.mockResolvedValue({ id: 998, name: 'parent-acc (Copy)' })
    createSparkShadow.mockResolvedValue({ id: 999, name: 'parent-acc (Spark)' })
  })

  afterEach(() => {
    vi.unstubAllGlobals()
  })

  it('AccountActionMenu 的 duplicate 事件一键复制账号并刷新列表', async () => {
    const wrapper = mountView()
    await flushPromises()

    wrapper.findComponent(AccountActionMenu).vm.$emit('duplicate', { id: 42, name: 'parent-acc' })
    await flushPromises()

    expect(duplicateAccount).toHaveBeenCalledTimes(1)
    expect(duplicateAccount).toHaveBeenCalledWith(42)
    expect(showSuccess).toHaveBeenCalledWith('admin.accounts.duplicateSuccess')
    expect(listAccounts.mock.calls.length).toBeGreaterThan(1)
    wrapper.unmount()
  })

  it('同一账号复制请求未完成时忽略重复点击', async () => {
    let resolveDuplicate!: (account: { id: number; name: string }) => void
    duplicateAccount.mockImplementationOnce(() => new Promise(resolve => { resolveDuplicate = resolve }))
    const wrapper = mountView()
    await flushPromises()

    const menu = wrapper.findComponent(AccountActionMenu)
    menu.vm.$emit('duplicate', { id: 42, name: 'parent-acc' })
    menu.vm.$emit('duplicate', { id: 42, name: 'parent-acc' })
    await flushPromises()

    expect(duplicateAccount).toHaveBeenCalledTimes(1)
    resolveDuplicate({ id: 998, name: 'parent-acc (Copy)' })
    await flushPromises()
    wrapper.unmount()
  })

  it('复制失败时显示后端错误', async () => {
    const consoleError = vi.spyOn(console, 'error').mockImplementation(() => {})
    duplicateAccount.mockRejectedValueOnce(new Error('duplicate failed'))
    const wrapper = mountView()
    await flushPromises()

    wrapper.findComponent(AccountActionMenu).vm.$emit('duplicate', { id: 42, name: 'parent-acc' })
    await flushPromises()

    expect(showError).toHaveBeenCalledWith('duplicate failed')
    consoleError.mockRestore()
    wrapper.unmount()
  })

  it('AccountActionMenu 的 create-spark-shadow 事件触发 createSparkShadow API + 成功提示', async () => {
    const wrapper = mountView()
    await flushPromises()

    const menu = wrapper.findComponent(AccountActionMenu)
    expect(menu.exists()).toBe(true)

    menu.vm.$emit('create-spark-shadow', { id: 42, name: 'parent-acc' })
    await flushPromises()

    // 不再用原生 confirm,改用应用内 ConfirmDialog:先弹出,点确认才调 API
    const dialog = wrapper.findAllComponents(ConfirmDialog).find(d => d.props('show'))
    expect(dialog).toBeTruthy()
    dialog?.vm.$emit('confirm')
    await flushPromises()

    expect(createSparkShadow).toHaveBeenCalledTimes(1)
    expect(createSparkShadow).toHaveBeenCalledWith(42, { name: 'parent-acc (Spark)' })
    expect(showSuccess).toHaveBeenCalledWith('admin.accounts.createSparkShadowSuccess')
    wrapper.unmount()
  })

  it('用户取消确认时不调用 API', async () => {
    const wrapper = mountView()
    await flushPromises()

    wrapper.findComponent(AccountActionMenu).vm.$emit('create-spark-shadow', { id: 42, name: 'parent-acc' })
    await flushPromises()

    // 弹出 ConfirmDialog 后点取消,不应调用 API
    const dialog = wrapper.findAllComponents(ConfirmDialog).find(d => d.props('show'))
    expect(dialog).toBeTruthy()
    dialog?.vm.$emit('cancel')
    await flushPromises()

    expect(createSparkShadow).not.toHaveBeenCalled()
    wrapper.unmount()
  })
})

// 账号行展示
const mountViewWithRow = () =>
  mount(AccountsView, {
    global: {
      stubs: {
        AppLayout: { template: '<div><slot /></div>' },
        TablePageLayout: {
          template: '<div><slot name="filters" /><slot name="table" /><slot name="pagination" /></div>'
        },
        // 使用能透传 row 数据的自定义 DataTable stub，以便渲染 cell 插槽
        DataTable: {
          props: ['data', 'columns', 'loading'],
          template: `<div>
            <div v-for="(row, idx) in (data || [])" :key="idx">
              <slot name="cell-name" :row="row" :value="row.name" />
              <slot name="cell-platform_type" :row="row" />
            </div>
          </div>`
        },
        Pagination: true,
        ConfirmDialog: true,
        AccountTableActions: { template: '<div><slot name="beforeCreate" /><slot name="after" /></div>' },
        AccountTableFilters: { template: '<div></div>' },
        AccountBulkActionsBar: true,
        AccountActionMenu: true,
        ImportDataModal: true,
        ReAuthAccountModal: true,
        AccountTestModal: true,
        AccountStatsModal: true,
        ScheduledTestsPanel: true,
        SyncFromCrsModal: true,
        TempUnschedStatusModal: true,
        ErrorPassthroughRulesModal: true,
        TLSFingerprintProfilesModal: true,
        CreateAccountModal: true,
        EditAccountModal: true,
        BulkEditAccountModal: true,
        PlatformTypeBadge: true,
        AccountCapacityCell: true,
        AccountStatusIndicator: true,
        AccountTodayStatsCell: true,
        AccountGroupsCell: true,
        AccountUsageCell: true,
        Icon: true
      }
    }
  })

describe('admin AccountsView — 账号行展示', () => {
  beforeEach(() => {
    localStorage.clear()
    for (const fn of [listAccounts, listWithEtag, getBatchTodayStats, getAllProxies, getAllGroups, duplicateAccount, createSparkShadow, showSuccess, showError]) {
      fn.mockReset()
    }
    listWithEtag.mockResolvedValue({ notModified: true, etag: null, data: null })
    getBatchTodayStats.mockResolvedValue({ stats: {} })
    getAllProxies.mockResolvedValue([])
    getAllGroups.mockResolvedValue([])
    vi.stubGlobal('confirm', vi.fn(() => true))
  })

  afterEach(() => {
    vi.useRealTimers()
    vi.restoreAllMocks()
    vi.unstubAllGlobals()
  })

  it('影子行 email 单元格显示 parent_email，PlatformTypeBadge 接收 parent_plan_type/parent_privacy_mode', async () => {
    const shadowAccount = {
      id: 100,
      name: '影子账号',
      platform: 'openai',
      type: 'oauth',
      parent_account_id: 1,
      parent_email: 'parent@example.com',
      parent_plan_type: 'plus',
      parent_privacy_mode: 'false',
      parent_subscription_expires_at: '2027-01-01T00:00:00Z',
      parent_chatgpt_account_id: 'chatgpt-abc123',
    }

    listAccounts.mockResolvedValue({ items: [shadowAccount], total: 1, page: 1, page_size: 20, pages: 1 })

    const wrapper = mountViewWithRow()
    await flushPromises()

    // 1. email 单元格通过 OR 兜底渲染 parent_email
    expect(wrapper.text()).toContain('parent@example.com')

    // 2. PlatformTypeBadge 收到 parent_plan_type 和 parent_privacy_mode
    const badge = wrapper.findComponent(PlatformTypeBadge)
    expect(badge.exists()).toBe(true)
    expect(badge.props('planType')).toBe('plus')
    expect(badge.props('privacyMode')).toBe('false')
    expect(badge.props('subscriptionExpiresAt')).toBe('2027-01-01T00:00:00Z')

    wrapper.unmount()
  })

  it('仅将具有安全 base_url 的 API Key 账号名称链接到站点主页', async () => {
    listAccounts.mockResolvedValue({
      items: [
        { id: 101, name: 'relay-account', platform: 'openai', type: 'apikey', credentials: { base_url: 'https://relay.example.com/api/v1/' } },
        { id: 102, name: 'oauth-account', platform: 'openai', type: 'oauth', credentials: { base_url: 'https://oauth.example.com/v1' } },
        { id: 103, name: 'invalid-url', platform: 'openai', type: 'apikey', credentials: { base_url: 'javascript:alert(1)' } },
      ],
      total: 3,
      page: 1,
      page_size: 20,
      pages: 1,
    })

    const wrapper = mountViewWithRow()
    await flushPromises()

    const links = wrapper.findAll('a')
    expect(links).toHaveLength(1)
    const [link] = links
    expect(link.text()).toBe('relay-account')
    expect(link.attributes()).toMatchObject({
      href: 'https://relay.example.com',
      target: '_blank',
      rel: 'noopener noreferrer',
    })
    expect(link.classes()).toEqual(expect.arrayContaining([
      'border-dotted',
      'text-gray-900',
      'dark:text-white',
    ]))
    expect(link.classes()).not.toContain('text-primary-600')
    const tooltip = wrapper.findComponent(HelpTooltip)
    expect(tooltip.props('content')).toBe('https://relay.example.com')
    expect(tooltip.props('widthClass')).toBe('w-max max-w-sm break-all')
    expect(tooltip.classes()).toEqual(expect.arrayContaining(['self-start']))
    expect(wrapper.text()).toContain('oauth-account')
    expect(wrapper.text()).toContain('invalid-url')

    wrapper.unmount()
  })

  it('prefers persisted Grok JWT tier over lagging billing/quota snapshots', async () => {
    const grokAccounts = [
      {
        id: 201,
        name: 'oauth-tier',
        platform: 'grok',
        type: 'oauth',
        credentials: { subscription_tier: 'FREE', plan_type: 'legacy' },
        extra: {
          grok_billing_snapshot: { plan: 'SuperGrok' },
          subscription_tier: 'BASIC',
        },
      },
      {
        id: 202,
        name: 'billing-tier',
        platform: 'grok',
        type: 'oauth',
        credentials: {},
        extra: {
          grok_billing_snapshot: { plan: 'SuperGrok Heavy' },
          subscription_tier: 'BASIC',
        },
      },
      {
        id: 203,
        name: 'quota-tier',
        platform: 'grok',
        type: 'oauth',
        credentials: { subscription_tier: 'FREE' },
        extra: {
          grok_quota_snapshot: { subscription_tier: 'SuperGrok' },
          subscription_tier: 'BASIC',
        },
      },
      {
        id: 204,
        name: 'extra-tier',
        platform: 'grok',
        type: 'oauth',
        credentials: { plan_type: 'SuperGrok' },
        extra: { subscription_tier: 'BASIC' },
      },
      {
        id: 205,
        name: 'legacy-tier',
        platform: 'grok',
        type: 'oauth',
        credentials: { plan_type: 'SuperGrok' },
      },
      {
        id: 206,
        name: 'supergrokpro-responses-quota',
        platform: 'grok',
        type: 'oauth',
        credentials: { subscription_tier: 'SuperGrokPro' },
        extra: {
          grok_billing_snapshot: { plan: 'SuperGrok' },
          grok_usage_snapshot: {
            model: 'grok-4.5',
            last_headers_seen_at: new Date().toISOString(),
            requests: { limit: 8300 },
            tokens: { limit: 53_000_000 },
          },
          grok_quota_snapshot: {
            model: 'grok-4.6',
            last_headers_seen_at: new Date().toISOString(),
            requests: { limit: 8300 },
            tokens: { limit: 53_000_000 },
          },
        },
      },
      {
        id: 207,
        name: 'supergrokpro-other-model-quota',
        platform: 'grok',
        type: 'oauth',
        credentials: { subscription_tier: 'SuperGrokPro' },
        extra: {
          grok_billing_snapshot: { plan: 'SuperGrok' },
          grok_usage_snapshot: {
            model: 'grok-4.6',
            last_headers_seen_at: new Date().toISOString(),
            requests: { limit: 8300 },
            tokens: { limit: 53_000_000 },
          },
        },
      },
      {
        id: 208,
        name: 'usage-over-legacy-quota',
        platform: 'grok',
        type: 'oauth',
        credentials: {},
        extra: {
          grok_usage_snapshot: { subscription_tier: 'SuperGrok' },
          grok_quota_snapshot: { subscription_tier: 'Free' },
        },
      },
      {
        id: 209,
        name: 'legacy-quota-alias',
        platform: 'grok',
        type: 'oauth',
        credentials: {},
        extra: { grok_quota_snapshot: { subscription_tier: 'SuperGrok' } },
      },
    ]

    listAccounts.mockResolvedValue({
      items: grokAccounts,
      total: grokAccounts.length,
      page: 1,
      page_size: 20,
      pages: 1,
    })

    const wrapper = mountViewWithRow()
    await flushPromises()

    const badges = wrapper.findAllComponents(PlatformTypeBadge)
    expect(badges.map((badge) => badge.props('planType'))).toEqual([
      'FREE',
      'SuperGrok Heavy',
      'FREE',
      'BASIC',
      'SuperGrok',
      'SuperGrok Heavy',
      'SuperGrok',
      'SuperGrok',
      'SuperGrok',
    ])

    wrapper.unmount()
  })

  it('skips malformed Grok plan fields and safely uses the next valid fallback', async () => {
    const grokAccounts = [
      {
        id: 210,
        name: 'legacy-fallback',
        platform: 'grok',
        type: 'oauth',
        credentials: {},
        extra: {
          grok_usage_snapshot: { subscription_tier: { name: 'SuperGrok Heavy' } },
          grok_quota_snapshot: { subscription_tier: 'SuperGrok' },
        },
      },
      {
        id: 211,
        name: 'credential-plan-fallback',
        platform: 'grok',
        type: 'oauth',
        credentials: { subscription_tier: 0, plan_type: 'SuperGrok Heavy' },
        extra: {
          grok_billing_snapshot: { plan: {} },
          grok_usage_snapshot: { subscription_tier: 1 },
          grok_quota_snapshot: { subscription_tier: [] },
          subscription_tier: '   ',
        },
      },
      {
        id: 212,
        name: 'no-valid-plan',
        platform: 'grok',
        type: 'oauth',
        credentials: { subscription_tier: {}, plan_type: 2 },
        parent_plan_type: [],
        extra: {
          grok_billing_snapshot: { plan: [] },
          grok_usage_snapshot: { subscription_tier: 1 },
          grok_quota_snapshot: { subscription_tier: {} },
          subscription_tier: null,
        },
      },
    ]

    listAccounts.mockResolvedValue({
      items: grokAccounts,
      total: grokAccounts.length,
      page: 1,
      page_size: 20,
      pages: 1,
    })

    const wrapper = mountViewWithRow()
    await flushPromises()

    expect(wrapper.findAllComponents(PlatformTypeBadge).map((badge) => badge.props('planType'))).toEqual([
      'SuperGrok',
      'SuperGrok Heavy',
      undefined,
    ])
    wrapper.unmount()
  })

  it('replaces a Grok row when auto refresh returns a changed canonical usage snapshot', async () => {
    vi.useFakeTimers()
    vi.spyOn(document, 'hidden', 'get').mockReturnValue(false)
    localStorage.setItem('account-auto-refresh', JSON.stringify({ enabled: true, interval_seconds: 5 }))

    const initialAccount = {
      id: 213,
      name: 'refresh-tier',
      platform: 'grok',
      type: 'oauth',
      extra: { grok_usage_snapshot: { subscription_tier: 'Free', status_code: 200 } },
    }
    const refreshedAccount = {
      ...initialAccount,
      extra: { grok_usage_snapshot: { subscription_tier: 'SuperGrok', status_code: 200 } },
    }
    listAccounts.mockResolvedValue({ items: [initialAccount], total: 1, page: 1, page_size: 20, pages: 1 })
    listWithEtag.mockResolvedValueOnce({
      notModified: false,
      etag: 'grok-snapshot-2',
      data: { items: [refreshedAccount], total: 1, page: 1, page_size: 20, pages: 1 },
    })

    const wrapper = mountViewWithRow()
    await flushPromises()
    expect(wrapper.findComponent(PlatformTypeBadge).props('planType')).toBe('Free')

    await vi.advanceTimersByTimeAsync(6000)
    await flushPromises()

    expect(listWithEtag).toHaveBeenCalledTimes(1)
    expect(wrapper.findComponent(PlatformTypeBadge).props('planType')).toBe('SuperGrok')
    wrapper.unmount()
  })
})
