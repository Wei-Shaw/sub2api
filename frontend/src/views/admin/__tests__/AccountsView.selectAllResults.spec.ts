import { beforeEach, describe, expect, it, vi REDACTED from 'vitest'
import { flushPromises, mount REDACTED from '@vue/test-utils'

import AccountsView from '../AccountsView.vue'

const {
  listAccounts,
  listWithEtag,
  getBatchTodayStats,
  getUpstreamBillingProbeSettings,
  getAllProxies,
  getAllGroups,
  showError
REDACTED = vi.hoisted(() => ({
  listAccounts: vi.fn(),
  listWithEtag: vi.fn(),
  getBatchTodayStats: vi.fn(),
  getUpstreamBillingProbeSettings: vi.fn(),
  getAllProxies: vi.fn(),
  getAllGroups: vi.fn(),
  showError: vi.fn()
REDACTED))

vi.mock('@/api/admin', () => ({
  adminAPI: {
    accounts: {
      list: listAccounts,
      listWithEtag,
      getBatchTodayStats,
      getUpstreamBillingProbeSettings,
      batchDelete: vi.fn(),
      batchClearError: vi.fn(),
      batchRefresh: vi.fn(),
      bulkUpdate: vi.fn()
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
    showError,
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

const makeAccounts = (count: number) => Array.from({ length: count REDACTED, (_, index) => ({
  id: index + 1,
  name: `account-${index + 1REDACTED`,
  platform: 'grok',
  type: 'oauth',
  status: 'active',
  schedulable: true,
  created_at: '2026-07-23T00:00:00Z',
  updated_at: '2026-07-23T00:00:00Z'
REDACTED))

const AccountBulkActionsBarStub = {
  props: ['selectedIds', 'totalResults', 'selectingAll', 'allResultsSelected'],
  emits: ['select-all-results', 'select-page', 'clear'],
  template: `
    <div>
      <span data-test="selected-count">{{ selectedIds.length REDACTEDREDACTED</span>
      <span data-test="total-results">{{ totalResults REDACTEDREDACTED</span>
      <span data-test="all-results-selected">{{ String(allResultsSelected) REDACTEDREDACTED</span>
      <button data-test="select-page" @click="$emit('select-page')">select page</button>
      <button data-test="select-all-results" @click="$emit('select-all-results')">select all</button>
      <button data-test="clear" @click="$emit('clear')">clear</button>
    </div>
  `
REDACTED

const AccountTableFiltersStub = {
  emits: ['change'],
  template: '<button data-test="change-filter" @click="$emit(\'change\')">change filter</button>'
REDACTED

const mountView = () => mount(AccountsView, {
  global: {
    stubs: {
      AppLayout: { template: '<div><slot /></div>' REDACTED,
      TablePageLayout: {
        template: '<div><slot name="filters" /><slot name="table" /><slot name="pagination" /></div>'
      REDACTED,
      DataTable: { props: ['data'], template: '<div data-test="data-table"></div>' REDACTED,
      Pagination: true,
      ConfirmDialog: true,
      AccountTableActions: { template: '<div><slot name="beforeCreate" /><slot name="after" /></div>' REDACTED,
      AccountTableFilters: AccountTableFiltersStub,
      AccountBulkActionsBar: AccountBulkActionsBarStub,
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

describe('admin AccountsView select all filtered results', () => {
  beforeEach(() => {
    localStorage.clear()
    listAccounts.mockReset()
    listWithEtag.mockReset()
    getBatchTodayStats.mockReset()
    getUpstreamBillingProbeSettings.mockReset()
    getAllProxies.mockReset()
    getAllGroups.mockReset()
    showError.mockReset()

    listWithEtag.mockResolvedValue({
      notModified: true,
      etag: null,
      data: null
    REDACTED)
    getBatchTodayStats.mockResolvedValue({ stats: {REDACTED REDACTED)
    getUpstreamBillingProbeSettings.mockResolvedValue({ enabled: true, interval_minutes: 30 REDACTED)
    getAllProxies.mockResolvedValue([])
    getAllGroups.mockResolvedValue([])
  REDACTED)

  it('selects all matching IDs in one commit and clears the selection when filters change', async () => {
    const allAccounts = makeAccounts(45)
    listAccounts.mockImplementation(async (_page: number, pageSize: number) => {
      if (pageSize === 1000) {
        return {
          items: allAccounts,
          total: 45,
          page: 1,
          page_size: 1000,
          pages: 1
        REDACTED
      REDACTED
      return {
        items: allAccounts.slice(0, 20),
        total: 45,
        page: 1,
        page_size: 20,
        pages: 3
      REDACTED
    REDACTED)

    const wrapper = mountView()
    await flushPromises()

    await wrapper.get('[data-test="select-all-results"]').trigger('click')
    await flushPromises()

    expect(wrapper.get('[data-test="selected-count"]').text()).toBe('45')
    expect(wrapper.get('[data-test="total-results"]').text()).toBe('45')
    expect(wrapper.get('[data-test="all-results-selected"]').text()).toBe('true')
    expect(listAccounts).toHaveBeenCalledWith(1, 1000, expect.objectContaining({
      lite: '1',
      include_scheduler_score: '0'
    REDACTED))

    await wrapper.get('[data-test="change-filter"]').trigger('click')

    expect(wrapper.get('[data-test="selected-count"]').text()).toBe('0')
    expect(wrapper.get('[data-test="all-results-selected"]').text()).toBe('false')
  REDACTED)

  it('keeps the original page selection when loading all results fails', async () => {
    const currentPage = makeAccounts(20)
    listAccounts.mockImplementation(async (_page: number, pageSize: number) => {
      if (pageSize === 1000) {
        throw new Error('load all failed')
      REDACTED
      return {
        items: currentPage,
        total: 45,
        page: 1,
        page_size: 20,
        pages: 3
      REDACTED
    REDACTED)

    const wrapper = mountView()
    await flushPromises()

    await wrapper.get('[data-test="select-page"]').trigger('click')
    expect(wrapper.get('[data-test="selected-count"]').text()).toBe('20')

    await wrapper.get('[data-test="select-all-results"]').trigger('click')
    await flushPromises()

    expect(wrapper.get('[data-test="selected-count"]').text()).toBe('20')
    expect(wrapper.get('[data-test="all-results-selected"]').text()).toBe('false')
    expect(showError).toHaveBeenCalledWith('admin.accounts.bulkActions.selectAllFailed')
  REDACTED)
REDACTED)
