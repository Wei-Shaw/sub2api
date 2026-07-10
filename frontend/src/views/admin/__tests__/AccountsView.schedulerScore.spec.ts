import { beforeEach, describe, expect, it, vi REDACTED from 'vitest'
import { flushPromises, mount REDACTED from '@vue/test-utils'

import AccountsView from '../AccountsView.vue'

const {
  listAccounts,
  listWithEtag,
  getBatchTodayStats,
  getAllProxies,
  getAllGroups
REDACTED = vi.hoisted(() => ({
  listAccounts: vi.fn(),
  listWithEtag: vi.fn(),
  getBatchTodayStats: vi.fn(),
  getAllProxies: vi.fn(),
  getAllGroups: vi.fn()
REDACTED))

vi.mock('@/api/admin', () => ({
  adminAPI: {
    accounts: {
      list: listAccounts,
      listWithEtag,
      getBatchTodayStats,
      delete: vi.fn(),
      batchClearError: vi.fn(),
      batchRefresh: vi.fn(),
      toggleSchedulable: vi.fn()
    REDACTED,
    proxies: {
      getAll: getAllProxies
    REDACTED,
    groups: {
      getAll: getAllGroups
    REDACTED
  REDACTED
REDACTED))

vi.mock('@/stores/app', () => ({
  useAppStore: () => ({
    showError: vi.fn(),
    showSuccess: vi.fn(),
    showInfo: vi.fn()
  REDACTED)
REDACTED))

vi.mock('@/stores/auth', () => ({
  useAuthStore: () => ({
    token: 'test-token'
  REDACTED)
REDACTED))

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return {
    ...actual,
    useI18n: () => ({
      t: (key: string) => key
    REDACTED)
  REDACTED
REDACTED)

// Render the scheduler-score cell slot for every row so the fallback logic is observable.
const DataTableStub = {
  props: ['columns', 'data'],
  template: `
    <div data-test="data-table">
      <div v-for="row in data" :key="row.id" :data-test="'scheduler-score-' + row.id">
        <slot name="cell-scheduler_score" :row="row" />
      </div>
    </div>
  `
REDACTED

function mountView() {
  return mount(AccountsView, {
    global: {
      stubs: {
        AppLayout: { template: '<div><slot /></div>' REDACTED,
        TablePageLayout: {
          template: '<div><slot name="filters" /><slot name="table" /><slot name="pagination" /></div>'
        REDACTED,
        DataTable: DataTableStub,
        HelpTooltip: true,
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
REDACTED

const baseAccount = {
  platform: 'openai',
  type: 'apikey',
  status: 'active',
  schedulable: true,
  concurrency: 1,
  priority: 0,
  error_message: null,
  last_used_at: null,
  expires_at: null,
  auto_pause_on_expired: false,
  created_at: '2026-01-01T00:00:00Z',
  updated_at: '2026-01-01T00:00:00Z'
REDACTED

describe('admin AccountsView scheduler score column', () => {
  beforeEach(() => {
    localStorage.clear()

    listAccounts.mockReset()
    listWithEtag.mockReset()
    getBatchTodayStats.mockReset()
    getAllProxies.mockReset()
    getAllGroups.mockReset()

    listAccounts.mockResolvedValue({
      items: [
        {
          ...baseAccount,
          id: 1,
          name: 'ungrouped-openai',
          // 未分组账号：后端只返回基础分（scheduler_score），无分组维度分数
          scheduler_score: {
            base_score: 1.234567,
            sticky_score: 0,
            sticky_weighted_enabled: false
          REDACTED
        REDACTED,
        {
          ...baseAccount,
          id: 2,
          name: 'grouped-openai',
          scheduler_score: {
            base_score: 2,
            sticky_score: 3,
            sticky_weighted_enabled: true
          REDACTED,
          scheduler_scores: [
            {
              group_id: 5,
              group_name: 'group-five',
              base_score: 2,
              sticky_score: 3,
              sticky_weighted_enabled: true
            REDACTED
          ]
        REDACTED,
        {
          ...baseAccount,
          id: 3,
          name: 'no-score',
          platform: 'anthropic'
        REDACTED
      ],
      total: 3,
      page: 1,
      page_size: 20,
      pages: 1
    REDACTED)
    listWithEtag.mockResolvedValue({
      notModified: true,
      etag: null,
      data: null
    REDACTED)
    getBatchTodayStats.mockResolvedValue({ stats: {REDACTED REDACTED)
    getAllProxies.mockResolvedValue([])
    getAllGroups.mockResolvedValue([])
  REDACTED)

  it('falls back to the base score for ungrouped accounts instead of showing a dash', async () => {
    const wrapper = mountView()
    await flushPromises()

    expect(listAccounts.mock.calls[0]?.[2]).toEqual(expect.objectContaining({
      include_scheduler_score: '0'
    REDACTED))

    const ungroupedCell = wrapper.find('[data-test="scheduler-score-1"]')
    expect(ungroupedCell.exists()).toBe(true)
    expect(ungroupedCell.text()).toContain('1.234567')
    expect(ungroupedCell.text()).toContain('admin.accounts.schedulerScore.ungrouped')
    expect(ungroupedCell.text()).not.toBe('-')
  REDACTED)

  it('renders per-group scores for grouped accounts', async () => {
    const wrapper = mountView()
    await flushPromises()

    const groupedCell = wrapper.find('[data-test="scheduler-score-2"]')
    expect(groupedCell.exists()).toBe(true)
    expect(groupedCell.text()).toContain('group-five')
    expect(groupedCell.text()).toContain('2')
  REDACTED)

  it('keeps scheduler score hidden for old saved column settings until the admin opts in again', async () => {
    localStorage.setItem('account-hidden-columns', JSON.stringify(['today_stats']))

    mountView()
    await flushPromises()

    expect(listAccounts.mock.calls[0]?.[2]).toEqual(expect.objectContaining({
      include_scheduler_score: '0'
    REDACTED))
    expect(JSON.parse(localStorage.getItem('account-hidden-columns') || '[]')).toContain('scheduler_score')
  REDACTED)

  it('requests scheduler scores when the migrated column settings explicitly show the column', async () => {
    localStorage.setItem('account-hidden-columns', JSON.stringify(['today_stats']))
    localStorage.setItem('account-hidden-columns-version', 'scheduler-score-hidden-by-default')

    mountView()
    await flushPromises()

    expect(listAccounts.mock.calls[0]?.[2]).toEqual(expect.objectContaining({
      include_scheduler_score: '1'
    REDACTED))
  REDACTED)

  it('still shows a dash when no scheduler score is available', async () => {
    const wrapper = mountView()
    await flushPromises()

    const emptyCell = wrapper.find('[data-test="scheduler-score-3"]')
    expect(emptyCell.exists()).toBe(true)
    expect(emptyCell.text()).toBe('-')
  REDACTED)
REDACTED)
