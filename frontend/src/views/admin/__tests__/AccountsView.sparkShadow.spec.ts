import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'

import AccountsView from '../AccountsView.vue'
import AccountActionMenu from '@/components/admin/account/AccountActionMenu.vue'
import PlatformTypeBadge from '@/components/common/PlatformTypeBadge.vue'
import ConfirmDialog from '@/components/common/ConfirmDialog.vue'

// 外审 F2:AccountActionMenu emit 'create-spark-shadow',但 AccountsView 此前未监听,
// 导致按钮点击无效。本测试通过真实组件引用 emit 该事件,断言父页面接线调用 API。
const {
  listAccounts,
  listWithEtag,
  getBatchTodayStats,
  getAllProxies,
  getAllGroups,
  createSparkShadow,
  showSuccess,
  showError
} = vi.hoisted(() => ({
  listAccounts: vi.fn(),
  listWithEtag: vi.fn(),
  getBatchTodayStats: vi.fn(),
  getAllProxies: vi.fn(),
  getAllGroups: vi.fn(),
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
    for (const fn of [listAccounts, listWithEtag, getBatchTodayStats, getAllProxies, getAllGroups, createSparkShadow, showSuccess, showError]) {
      fn.mockReset()
    }
    listAccounts.mockResolvedValue({ items: [], total: 0, page: 1, page_size: 20, pages: 0 })
    listWithEtag.mockResolvedValue({ notModified: true, etag: null, data: null })
    getBatchTodayStats.mockResolvedValue({ stats: {} })
    getAllProxies.mockResolvedValue([])
    getAllGroups.mockResolvedValue([])
    createSparkShadow.mockResolvedValue({ id: 999, name: 'parent-acc (Spark)' })
  })

  afterEach(() => {
    vi.unstubAllGlobals()
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

// Task 6: 影子行 parent_* OR 兜底展示
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

describe('admin AccountsView — 影子行 parent_* OR 兜底展示', () => {
  beforeEach(() => {
    localStorage.clear()
    for (const fn of [listAccounts, listWithEtag, getBatchTodayStats, getAllProxies, getAllGroups, createSparkShadow, showSuccess, showError]) {
      fn.mockReset()
    }
    listWithEtag.mockResolvedValue({ notModified: true, etag: null, data: null })
    getBatchTodayStats.mockResolvedValue({ stats: {} })
    getAllProxies.mockResolvedValue([])
    getAllGroups.mockResolvedValue([])
    vi.stubGlobal('confirm', vi.fn(() => true))
  })

  afterEach(() => {
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

  it('passes fresh Grok billing and quota snapshots before stale credential fallbacks', async () => {
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
      'SuperGrok',
      'SuperGrok Heavy',
      'SuperGrok',
      'BASIC',
      'SuperGrok',
    ])

    wrapper.unmount()
  })
})
