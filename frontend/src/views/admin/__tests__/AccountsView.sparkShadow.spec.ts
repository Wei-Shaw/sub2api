import { afterEach, beforeEach, describe, expect, it, vi REDACTED from 'vitest'
import { flushPromises, mount REDACTED from '@vue/test-utils'

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
REDACTED = vi.hoisted(() => ({
  listAccounts: vi.fn(),
  listWithEtag: vi.fn(),
  getBatchTodayStats: vi.fn(),
  getAllProxies: vi.fn(),
  getAllGroups: vi.fn(),
  duplicateAccount: vi.fn(),
  createSparkShadow: vi.fn(),
  showSuccess: vi.fn(),
  showError: vi.fn()
REDACTED))

vi.mock('@/api/admin', () => ({
  adminAPI: {
    accounts: {
      list: listAccounts,
      listWithEtag,
      getBatchTodayStats,
      duplicate: duplicateAccount,
      getUpstreamBillingProbeSettings: vi.fn().mockResolvedValue({ enabled: true, interval_minutes: 30 REDACTED),
      createSparkShadow,
      delete: vi.fn(),
      batchClearError: vi.fn(),
      batchRefresh: vi.fn(),
      toggleSchedulable: vi.fn()
    REDACTED,
    proxies: { getAll: getAllProxies REDACTED,
    groups: { getAll: getAllGroups REDACTED
  REDACTED
REDACTED))

vi.mock('@/stores/app', () => ({
  useAppStore: () => ({ showError, showSuccess, showInfo: vi.fn() REDACTED)
REDACTED))

vi.mock('@/stores/auth', () => ({
  useAuthStore: () => ({ token: 'test-token' REDACTED)
REDACTED))

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return {
    ...actual,
    useI18n: () => ({ t: (key: string) => key REDACTED)
  REDACTED
REDACTED)

const mountView = () =>
  mount(AccountsView, {
    global: {
      stubs: {
        AppLayout: { template: '<div><slot /></div>' REDACTED,
        TablePageLayout: {
          template: '<div><slot name="filters" /><slot name="table" /><slot name="pagination" /></div>'
        REDACTED,
        DataTable: true,
        Pagination: true,
        ConfirmDialog: true,
        AccountTableActions: { template: '<div><slot name="beforeCreate" /><slot name="after" /></div>' REDACTED,
        AccountTableFilters: { template: '<div></div>' REDACTED,
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
      REDACTED
    REDACTED
  REDACTED)

describe('admin AccountsView — 外审 F2:spark 影子创建接线', () => {
  beforeEach(() => {
    localStorage.clear()
    for (const fn of [listAccounts, listWithEtag, getBatchTodayStats, getAllProxies, getAllGroups, duplicateAccount, createSparkShadow, showSuccess, showError]) {
      fn.mockReset()
    REDACTED
    listAccounts.mockResolvedValue({ items: [], total: 0, page: 1, page_size: 20, pages: 0 REDACTED)
    listWithEtag.mockResolvedValue({ notModified: true, etag: null, data: null REDACTED)
    getBatchTodayStats.mockResolvedValue({ stats: {REDACTED REDACTED)
    getAllProxies.mockResolvedValue([])
    getAllGroups.mockResolvedValue([])
    duplicateAccount.mockResolvedValue({ id: 998, name: 'parent-acc (Copy)' REDACTED)
    createSparkShadow.mockResolvedValue({ id: 999, name: 'parent-acc (Spark)' REDACTED)
  REDACTED)

  afterEach(() => {
    vi.unstubAllGlobals()
  REDACTED)

  it('AccountActionMenu 的 duplicate 事件一键复制账号并刷新列表', async () => {
    const wrapper = mountView()
    await flushPromises()

    wrapper.findComponent(AccountActionMenu).vm.$emit('duplicate', { id: 42, name: 'parent-acc' REDACTED)
    await flushPromises()

    expect(duplicateAccount).toHaveBeenCalledTimes(1)
    expect(duplicateAccount).toHaveBeenCalledWith(42)
    expect(showSuccess).toHaveBeenCalledWith('admin.accounts.duplicateSuccess')
    expect(listAccounts.mock.calls.length).toBeGreaterThan(1)
    wrapper.unmount()
  REDACTED)

  it('同一账号复制请求未完成时忽略重复点击', async () => {
    let resolveDuplicate!: (account: { id: number; name: string REDACTED) => void
    duplicateAccount.mockImplementationOnce(() => new Promise(resolve => { resolveDuplicate = resolve REDACTED))
    const wrapper = mountView()
    await flushPromises()

    const menu = wrapper.findComponent(AccountActionMenu)
    menu.vm.$emit('duplicate', { id: 42, name: 'parent-acc' REDACTED)
    menu.vm.$emit('duplicate', { id: 42, name: 'parent-acc' REDACTED)
    await flushPromises()

    expect(duplicateAccount).toHaveBeenCalledTimes(1)
    resolveDuplicate({ id: 998, name: 'parent-acc (Copy)' REDACTED)
    await flushPromises()
    wrapper.unmount()
  REDACTED)

  it('复制失败时显示后端错误', async () => {
    const consoleError = vi.spyOn(console, 'error').mockImplementation(() => {REDACTED)
    duplicateAccount.mockRejectedValueOnce(new Error('duplicate failed'))
    const wrapper = mountView()
    await flushPromises()

    wrapper.findComponent(AccountActionMenu).vm.$emit('duplicate', { id: 42, name: 'parent-acc' REDACTED)
    await flushPromises()

    expect(showError).toHaveBeenCalledWith('duplicate failed')
    consoleError.mockRestore()
    wrapper.unmount()
  REDACTED)

  it('AccountActionMenu 的 create-spark-shadow 事件触发 createSparkShadow API + 成功提示', async () => {
    const wrapper = mountView()
    await flushPromises()

    const menu = wrapper.findComponent(AccountActionMenu)
    expect(menu.exists()).toBe(true)

    menu.vm.$emit('create-spark-shadow', { id: 42, name: 'parent-acc' REDACTED)
    await flushPromises()

    // 不再用原生 confirm,改用应用内 ConfirmDialog:先弹出,点确认才调 API
    const dialog = wrapper.findAllComponents(ConfirmDialog).find(d => d.props('show'))
    expect(dialog).toBeTruthy()
    dialog?.vm.$emit('confirm')
    await flushPromises()

    expect(createSparkShadow).toHaveBeenCalledTimes(1)
    expect(createSparkShadow).toHaveBeenCalledWith(42, { name: 'parent-acc (Spark)' REDACTED)
    expect(showSuccess).toHaveBeenCalledWith('admin.accounts.createSparkShadowSuccess')
    wrapper.unmount()
  REDACTED)

  it('用户取消确认时不调用 API', async () => {
    const wrapper = mountView()
    await flushPromises()

    wrapper.findComponent(AccountActionMenu).vm.$emit('create-spark-shadow', { id: 42, name: 'parent-acc' REDACTED)
    await flushPromises()

    // 弹出 ConfirmDialog 后点取消,不应调用 API
    const dialog = wrapper.findAllComponents(ConfirmDialog).find(d => d.props('show'))
    expect(dialog).toBeTruthy()
    dialog?.vm.$emit('cancel')
    await flushPromises()

    expect(createSparkShadow).not.toHaveBeenCalled()
    wrapper.unmount()
  REDACTED)
REDACTED)

// 账号行展示
const mountViewWithRow = () =>
  mount(AccountsView, {
    global: {
      stubs: {
        AppLayout: { template: '<div><slot /></div>' REDACTED,
        TablePageLayout: {
          template: '<div><slot name="filters" /><slot name="table" /><slot name="pagination" /></div>'
        REDACTED,
        // 使用能透传 row 数据的自定义 DataTable stub，以便渲染 cell 插槽
        DataTable: {
          props: ['data', 'columns', 'loading'],
          template: `<div>
            <div v-for="(row, idx) in (data || [])" :key="idx">
              <slot name="cell-name" :row="row" :value="row.name" />
              <slot name="cell-platform_type" :row="row" />
            </div>
          </div>`
        REDACTED,
        Pagination: true,
        ConfirmDialog: true,
        AccountTableActions: { template: '<div><slot name="beforeCreate" /><slot name="after" /></div>' REDACTED,
        AccountTableFilters: { template: '<div></div>' REDACTED,
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
      REDACTED
    REDACTED
  REDACTED)

describe('admin AccountsView — 账号行展示', () => {
  beforeEach(() => {
    localStorage.clear()
    for (const fn of [listAccounts, listWithEtag, getBatchTodayStats, getAllProxies, getAllGroups, duplicateAccount, createSparkShadow, showSuccess, showError]) {
      fn.mockReset()
    REDACTED
    listWithEtag.mockResolvedValue({ notModified: true, etag: null, data: null REDACTED)
    getBatchTodayStats.mockResolvedValue({ stats: {REDACTED REDACTED)
    getAllProxies.mockResolvedValue([])
    getAllGroups.mockResolvedValue([])
    vi.stubGlobal('confirm', vi.fn(() => true))
  REDACTED)

  afterEach(() => {
    vi.unstubAllGlobals()
  REDACTED)

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
    REDACTED

    listAccounts.mockResolvedValue({ items: [shadowAccount], total: 1, page: 1, page_size: 20, pages: 1 REDACTED)

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
  REDACTED)

  it('仅将具有安全 base_url 的 API Key 账号名称链接到站点主页', async () => {
    listAccounts.mockResolvedValue({
      items: [
        { id: 101, name: 'relay-account', platform: 'openai', type: 'apikey', credentials: { base_url: 'https://relay.example.com/api/v1/' REDACTED REDACTED,
        { id: 102, name: 'oauth-account', platform: 'openai', type: 'oauth', credentials: { base_url: 'https://oauth.example.com/v1' REDACTED REDACTED,
        { id: 103, name: 'invalid-url', platform: 'openai', type: 'apikey', credentials: { base_url: 'javascript:alert(1)' REDACTED REDACTED,
      ],
      total: 3,
      page: 1,
      page_size: 20,
      pages: 1,
    REDACTED)

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
    REDACTED)
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
  REDACTED)

  it('passes fresh Grok billing and quota snapshots before stale credential fallbacks', async () => {
    const grokAccounts = [
      {
        id: 201,
        name: 'oauth-tier',
        platform: 'grok',
        type: 'oauth',
        credentials: { subscription_tier: 'FREE', plan_type: 'legacy' REDACTED,
        extra: {
          grok_billing_snapshot: { plan: 'SuperGrok' REDACTED,
          subscription_tier: 'BASIC',
        REDACTED,
      REDACTED,
      {
        id: 202,
        name: 'billing-tier',
        platform: 'grok',
        type: 'oauth',
        credentials: {REDACTED,
        extra: {
          grok_billing_snapshot: { plan: 'SuperGrok Heavy' REDACTED,
          subscription_tier: 'BASIC',
        REDACTED,
      REDACTED,
      {
        id: 203,
        name: 'quota-tier',
        platform: 'grok',
        type: 'oauth',
        credentials: { subscription_tier: 'FREE' REDACTED,
        extra: {
          grok_quota_snapshot: { subscription_tier: 'SuperGrok' REDACTED,
          subscription_tier: 'BASIC',
        REDACTED,
      REDACTED,
      {
        id: 204,
        name: 'extra-tier',
        platform: 'grok',
        type: 'oauth',
        credentials: { plan_type: 'SuperGrok' REDACTED,
        extra: { subscription_tier: 'BASIC' REDACTED,
      REDACTED,
      {
        id: 205,
        name: 'legacy-tier',
        platform: 'grok',
        type: 'oauth',
        credentials: { plan_type: 'SuperGrok' REDACTED,
      REDACTED,
    ]

    listAccounts.mockResolvedValue({
      items: grokAccounts,
      total: grokAccounts.length,
      page: 1,
      page_size: 20,
      pages: 1,
    REDACTED)

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
  REDACTED)
REDACTED)
