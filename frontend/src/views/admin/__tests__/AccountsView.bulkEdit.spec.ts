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
  probeUpstreamBillingBatch
REDACTED = vi.hoisted(() => ({
  listAccounts: vi.fn(),
  listWithEtag: vi.fn(),
  getBatchTodayStats: vi.fn(),
  getUpstreamBillingProbeSettings: vi.fn(),
  getAllProxies: vi.fn(),
  getAllGroups: vi.fn(),
  probeUpstreamBillingBatch: vi.fn()
REDACTED))

vi.mock('@/api/admin', () => ({
  adminAPI: {
    accounts: {
      list: listAccounts,
      listWithEtag,
      getBatchTodayStats,
      getUpstreamBillingProbeSettings,
      delete: vi.fn(),
      batchClearError: vi.fn(),
      batchRefresh: vi.fn(),
      probeUpstreamBillingBatch,
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

const DataTableStub = {
  props: ['columns', 'data'],
  template: `
    <div data-test="data-table">
      <span v-for="column in columns" :key="column.key" data-test="column-key">{{ column.key REDACTEDREDACTED</span>
      <div v-for="row in data" :key="row.id">
        <div data-test="select-row"><slot name="cell-select" :row="row" /></div>
        <slot name="cell-created_at" :value="row.created_at" :row="row" />
      </div>
    </div>
  `
REDACTED

const AccountBulkActionsBarStub = {
  props: ['selectedIds'],
  emits: ['edit-filtered', 'probe-upstream-billing'],
  template: `
    <div>
      <button data-test="edit-filtered" @click="$emit('edit-filtered')">edit filtered</button>
      <button data-test="probe-upstream-billing" @click="$emit('probe-upstream-billing')">probe</button>
    </div>
  `
REDACTED

const PaginationStub = {
  emits: ['update:page'],
  template: '<button data-test="next-page" @click="$emit(\'update:page\', 2)">next</button>'
REDACTED

const BulkEditAccountModalStub = {
  props: ['show', 'target'],
  template: '<div data-test="bulk-edit-modal" :data-show="String(show)" :data-target-mode="target?.mode ?? \'\'"></div>'
REDACTED

describe('admin AccountsView bulk edit scope', () => {
  beforeEach(() => {
    localStorage.clear()

    listAccounts.mockReset()
    listWithEtag.mockReset()
    getBatchTodayStats.mockReset()
    getUpstreamBillingProbeSettings.mockReset()
    getAllProxies.mockReset()
    getAllGroups.mockReset()
    probeUpstreamBillingBatch.mockReset()

    listAccounts.mockResolvedValue({
      items: [],
      total: 0,
      page: 1,
      page_size: 20,
      pages: 0
    REDACTED)
    listWithEtag.mockResolvedValue({
      notModified: true,
      etag: null,
      data: null
    REDACTED)
    getBatchTodayStats.mockResolvedValue({ stats: {REDACTED REDACTED)
    getUpstreamBillingProbeSettings.mockResolvedValue({ enabled: true, interval_minutes: 30 REDACTED)
    getAllProxies.mockResolvedValue([])
    getAllGroups.mockResolvedValue([])
    probeUpstreamBillingBatch.mockResolvedValue([])
  REDACTED)

  it('opens bulk edit in filtered-results mode from the bulk actions dropdown', async () => {
    const wrapper = mount(AccountsView, {
      global: {
        stubs: {
          AppLayout: { template: '<div><slot /></div>' REDACTED,
          TablePageLayout: {
            template: '<div><slot name="filters" /><slot name="table" /><slot name="pagination" /></div>'
          REDACTED,
          DataTable: DataTableStub,
          Pagination: true,
          ConfirmDialog: true,
          AccountTableActions: { template: '<div><slot name="beforeCreate" /><slot name="after" /></div>' REDACTED,
          AccountTableFilters: { template: '<div></div>' REDACTED,
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
          BulkEditAccountModal: BulkEditAccountModalStub,
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

    await flushPromises()
    await wrapper.get('[data-test="edit-filtered"]').trigger('click')
    await flushPromises()

    expect(wrapper.get('[data-test="bulk-edit-modal"]').attributes('data-show')).toBe('true')
    expect(wrapper.get('[data-test="bulk-edit-modal"]').attributes('data-target-mode')).toBe('filtered')
  REDACTED)

  it('renders the created_at column by default', async () => {
    listAccounts.mockResolvedValue({
      items: [
        {
          id: 1,
          name: 'test-account',
          platform: 'anthropic',
          type: 'oauth',
          status: 'active',
          schedulable: true,
          created_at: '2026-03-07T10:00:00Z',
          updated_at: '2026-03-07T10:00:00Z'
        REDACTED
      ],
      total: 1,
      page: 1,
      page_size: 20,
      pages: 1
    REDACTED)

    const wrapper = mount(AccountsView, {
      global: {
        stubs: {
          AppLayout: { template: '<div><slot /></div>' REDACTED,
          TablePageLayout: {
            template: '<div><slot name="filters" /><slot name="table" /><slot name="pagination" /></div>'
          REDACTED,
          DataTable: DataTableStub,
          Pagination: true,
          ConfirmDialog: true,
          AccountTableActions: { template: '<div><slot name="beforeCreate" /><slot name="after" /></div>' REDACTED,
          AccountTableFilters: { template: '<div></div>' REDACTED,
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
          BulkEditAccountModal: BulkEditAccountModalStub,
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

    await flushPromises()

    const columnKeys = wrapper.findAll('[data-test="column-key"]').map(node => node.text())
    expect(columnKeys).toContain('created_at')
    const columns = wrapper.getComponent(DataTableStub).props('columns') as Array<{ key: string; label: string; sortable: boolean REDACTED>
    expect(columns.find(column => column.key === 'created_at')).toMatchObject({
      label: 'admin.accounts.columns.createdAt',
      sortable: true
    REDACTED)
  REDACTED)

  it('passes the loaded global probe state to every upstream billing cell', async () => {
    listAccounts.mockResolvedValue({
      items: [
        {
          id: 1,
          name: 'upstream',
          platform: 'openai',
          type: 'apikey',
          status: 'active',
          schedulable: true,
          created_at: '2026-07-13T00:00:00Z',
          updated_at: '2026-07-13T00:00:00Z'
        REDACTED
      ],
      total: 1,
      page: 1,
      page_size: 20,
      pages: 1
    REDACTED)
    getUpstreamBillingProbeSettings.mockResolvedValue({ enabled: false, interval_minutes: 30 REDACTED)

    const wrapper = mount(AccountsView, {
      global: {
        stubs: {
          AppLayout: { template: '<div><slot /></div>' REDACTED,
          TablePageLayout: { template: '<div><slot name="table" /></div>' REDACTED,
          DataTable: {
            props: ['data'],
            template: '<div><div v-for="row in data" :key="row.id"><slot name="cell-upstream_billing_rate" :row="row" /></div></div>'
          REDACTED,
          UpstreamBillingRateCell: {
            props: ['globalProbeEnabled'],
            template: '<span data-test="upstream-billing-cell" :data-global-enabled="String(globalProbeEnabled)"></span>'
          REDACTED,
          Pagination: true,
          ConfirmDialog: true,
          AccountTableActions: true,
          AccountTableFilters: true,
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

    await flushPromises()

    expect(getUpstreamBillingProbeSettings).toHaveBeenCalledTimes(1)
    expect(wrapper.get('[data-test="upstream-billing-cell"]').attributes('data-global-enabled')).toBe('false')
  REDACTED)

  it('submits selected account IDs from every page for backend eligibility checks', async () => {
    const account = (id: number) => ({
      id,
      name: `account-${idREDACTED`,
      platform: 'openai',
      type: 'apikey',
      status: 'active',
      schedulable: true,
      created_at: '2026-07-13T00:00:00Z',
      updated_at: '2026-07-13T00:00:00Z'
    REDACTED)
    listAccounts
      .mockResolvedValueOnce({ items: [account(7)], total: 2, page: 1, page_size: 1, pages: 2 REDACTED)
      .mockResolvedValueOnce({ items: [account(11)], total: 2, page: 2, page_size: 1, pages: 2 REDACTED)

    const wrapper = mount(AccountsView, {
      global: {
        stubs: {
          AppLayout: { template: '<div><slot /></div>' REDACTED,
          TablePageLayout: { template: '<div><slot name="table" /><slot name="pagination" /></div>' REDACTED,
          DataTable: DataTableStub,
          Pagination: PaginationStub,
          ConfirmDialog: true,
          AccountTableActions: true,
          AccountTableFilters: true,
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
          BulkEditAccountModal: BulkEditAccountModalStub,
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

    await flushPromises()
    await wrapper.get('[data-test="select-row"] input').trigger('change')
    await wrapper.get('[data-test="next-page"]').trigger('click')
    await flushPromises()
    await wrapper.get('[data-test="select-row"] input').trigger('change')
    await wrapper.get('[data-test="probe-upstream-billing"]').trigger('click')
    await flushPromises()

    expect(probeUpstreamBillingBatch).toHaveBeenCalledWith([7, 11])
  REDACTED)

  it('reloads the server-sorted list after a batch probe changes a snapshot', async () => {
    localStorage.setItem('account-table-sort', JSON.stringify({ key: 'upstream_billing_rate', order: 'asc' REDACTED))
    const account = (id: number) => ({
      id,
      name: `account-${idREDACTED`,
      platform: 'openai',
      type: 'apikey',
      status: 'active',
      schedulable: true,
      created_at: '2026-07-13T00:00:00Z',
      updated_at: '2026-07-13T00:00:00Z'
    REDACTED)
    listAccounts
      .mockResolvedValueOnce({ items: [account(7)], total: 1, page: 1, page_size: 20, pages: 1 REDACTED)
      .mockResolvedValueOnce({ items: [account(7)], total: 1, page: 1, page_size: 20, pages: 1 REDACTED)
    probeUpstreamBillingBatch.mockResolvedValue([
      {
        account_id: 7,
        snapshot: {
          status: 'ok',
          data: { effective_rate_multiplier: 0.5 REDACTED,
          last_attempt_at: '2026-07-13T00:00:00Z',
          next_probe_at: '2026-07-13T00:30:00Z'
        REDACTED
      REDACTED
    ])

    const wrapper = mount(AccountsView, {
      global: {
        stubs: {
          AppLayout: { template: '<div><slot /></div>' REDACTED,
          TablePageLayout: { template: '<div><slot name="table" /></div>' REDACTED,
          DataTable: DataTableStub,
          AccountBulkActionsBar: AccountBulkActionsBarStub,
          AccountTableActions: true,
          AccountTableFilters: true,
          AccountActionMenu: true,
          Pagination: true,
          ConfirmDialog: true,
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
          BulkEditAccountModal: BulkEditAccountModalStub,
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

    await flushPromises()
    await wrapper.get('[data-test="select-row"] input').trigger('change')
    await wrapper.get('[data-test="probe-upstream-billing"]').trigger('click')
    await flushPromises()

    expect(probeUpstreamBillingBatch).toHaveBeenCalledWith([7])
    expect(listAccounts).toHaveBeenCalledTimes(2)
  REDACTED)
REDACTED)
