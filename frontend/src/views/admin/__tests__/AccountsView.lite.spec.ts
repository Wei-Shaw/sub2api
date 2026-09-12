import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { DOMWrapper, flushPromises, mount } from '@vue/test-utils'
import { defineComponent } from 'vue'

import AccountsView from '../AccountsView.vue'
import AccountActionMenu from '@/components/admin/account/AccountActionMenu.vue'

const {
  listAccounts,
  listWithEtag,
  getById,
  getBatchTodayStats,
  getUpstreamBillingProbeSettings,
  getAllProxies,
  getAllGroups,
  showError
} = vi.hoisted(() => ({
  listAccounts: vi.fn(),
  listWithEtag: vi.fn(),
  getById: vi.fn(),
  getBatchTodayStats: vi.fn(),
  getUpstreamBillingProbeSettings: vi.fn(),
  getAllProxies: vi.fn(),
  getAllGroups: vi.fn(),
  showError: vi.fn()
}))

vi.mock('@/api/admin', () => ({
  adminAPI: {
    accounts: {
      list: listAccounts,
      getById,
      listWithEtag,
      getBatchTodayStats,
      getUpstreamBillingProbeSettings,
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
  useAppStore: () => ({ showError, showSuccess: vi.fn(), showInfo: vi.fn() })
}))

vi.mock('@/stores/auth', () => ({
  useAuthStore: () => ({ token: 'test-token', isSimpleMode: false })
}))

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return { ...actual, useI18n: () => ({ t: (key: string) => key }) }
})

const DataTableStub = defineComponent({
  props: { data: { type: Array, default: () => [] } },
  template: `
    <div>
      <div v-for="row in data" :key="row.id">
        <slot name="cell-groups" :row="row" />
        <slot name="cell-actions" :row="row" />
      </div>
    </div>
  `
})

const AccountGroupsCellStub = defineComponent({
  props: { groups: { type: Array, default: () => [] } },
  template: '<span data-test="account-groups">{{ groups.map(group => group.name).join(",") }}</span>'
})

const EditAccountModalStub = defineComponent({
  props: { show: Boolean, account: { type: Object, default: null } },
  template: '<div data-test="edit-account">{{ show ? account?.name : "" }}</div>'
})

const AccountTestModalStub = defineComponent({
  props: { show: Boolean, account: { type: Object, default: null } },
  template: '<div data-test="test-account">{{ show ? account?.name : "" }}</div>'
})

const AccountStatsModalStub = defineComponent({
  props: { show: Boolean, account: { type: Object, default: null } },
  template: '<div data-test="stats-account">{{ show ? account?.name : "" }}</div>'
})

const VueDraggableStub = defineComponent({
  props: { modelValue: { type: Array, default: () => [] } },
  emits: ['update:modelValue', 'start', 'end'],
  methods: {
    simulateReorder() {
      this.$emit('start')
      this.$emit('update:modelValue', [...this.modelValue].reverse())
      this.$emit('end')
    }
  },
  template: `
    <div data-test="group-tabs-draggable">
      <slot />
      <button type="button" data-test="simulate-group-reorder" @click="simulateReorder" />
    </div>
  `
})

function mountView(stubActionMenu = true) {
  return mount(AccountsView, {
    attachTo: document.body,
    global: {
      stubs: {
        AppLayout: { template: '<div><slot /></div>' },
        TablePageLayout: { template: '<div><slot name="filters" /><slot name="table" /><slot name="pagination" /></div>' },
        DataTable: DataTableStub,
        AccountTableActions: { template: '<div><slot name="after" /></div>' },
        AccountTableFilters: true,
        AccountBulkActionsBar: true,
        Pagination: true,
        ConfirmDialog: true,
        AccountActionMenu: stubActionMenu,
        ImportDataModal: true,
        ReAuthAccountModal: true,
        AccountTestModal: AccountTestModalStub,
        AccountStatsModal: AccountStatsModalStub,
        ScheduledTestsPanel: true,
        SyncFromCrsModal: true,
        TempUnschedStatusModal: true,
        ErrorPassthroughRulesModal: true,
        TLSFingerprintProfilesModal: true,
        CreateAccountModal: true,
        EditAccountModal: EditAccountModalStub,
        BulkEditAccountModal: true,
        PlatformTypeBadge: true,
        AccountCapacityCell: true,
        AccountStatusIndicator: true,
        AccountTodayStatsCell: true,
        AccountGroupsCell: AccountGroupsCellStub,
        AccountUsageCell: true,
        UpstreamBillingRateCell: true,
        HelpTooltip: true,
        Icon: true,
        VueDraggable: VueDraggableStub,
        Teleport: stubActionMenu
      }
    }
  })
}

const listRow = {
  id: 42,
  name: 'compact row',
  platform: 'openai',
  type: 'oauth',
  status: 'active',
  schedulable: true,
  concurrency: 2,
  priority: 1,
  group_ids: [7],
  extra: {},
  credentials: {}
}

const fullAccount = {
  ...listRow,
  groups: [{ id: 7, name: 'codex', platform: 'openai' }],
  account_groups: [{ account_id: 42, group_id: 7 }],
  credentials: { api_key: 'redacted' },
  extra: { detail_only: true }
}

describe('admin AccountsView lite account list', () => {
  beforeEach(() => {
    localStorage.clear()
    listAccounts.mockReset().mockResolvedValue({ items: [listRow], total: 1, page: 1, page_size: 20, pages: 1 })
    listWithEtag.mockReset().mockResolvedValue({ notModified: true, etag: 'compact-etag', data: null })
    getById.mockReset().mockResolvedValue(fullAccount)
    getBatchTodayStats.mockReset().mockResolvedValue({ stats: {} })
    getUpstreamBillingProbeSettings.mockReset().mockResolvedValue({ enabled: true })
    getAllProxies.mockReset().mockResolvedValue([])
    getAllGroups.mockReset().mockResolvedValue([{ id: 7, name: 'codex', platform: 'openai' }])
    showError.mockReset()
  })

  afterEach(() => {
    vi.useRealTimers()
    vi.restoreAllMocks()
  })

  it('keeps lite=1 on the initial list request', async () => {
    const wrapper = mountView()
    await flushPromises()

    expect(listAccounts).toHaveBeenCalledWith(
      1,
      20,
      expect.objectContaining({ lite: '1' }),
      expect.objectContaining({ signal: expect.any(AbortSignal) })
    )
    wrapper.unmount()
  })

  it('loads a group list only after its tab is activated', async () => {
    const wrapper = mountView()
    await flushPromises()

    expect(listAccounts).toHaveBeenCalledTimes(1)
    const groupTab = wrapper.findAll('[role="tab"]')[1]
    expect(groupTab).toBeTruthy()

    await groupTab.trigger('click')
    await flushPromises()

    expect(listAccounts).toHaveBeenCalledTimes(2)
    expect(listAccounts).toHaveBeenLastCalledWith(
      1,
      20,
      expect.objectContaining({ group: '7', lite: '1' }),
      expect.objectContaining({ signal: expect.any(AbortSignal) })
    )
    wrapper.unmount()
  })

  it('restores the local group order while keeping boundary tabs fixed', async () => {
    localStorage.setItem('account-group-tab-order', JSON.stringify([9, 7]))
    getAllGroups.mockResolvedValue([
      { id: 7, name: 'first', platform: 'openai' },
      { id: 8, name: 'new', platform: 'openai' },
      { id: 9, name: 'last', platform: 'openai' }
    ])

    const wrapper = mountView()
    await flushPromises()

    expect(wrapper.findAll('[role="tab"]').map((tab) => tab.text().trim())).toEqual([
      'admin.accounts.allGroups',
      'last',
      'first',
      'new',
      'admin.accounts.ungroupedTab'
    ])
    wrapper.unmount()
  })

  it('persists a dragged group order while keeping boundary tabs fixed', async () => {
    getAllGroups.mockResolvedValue([
      { id: 7, name: 'first', platform: 'openai' },
      { id: 8, name: 'middle', platform: 'openai' },
      { id: 9, name: 'last', platform: 'openai' }
    ])

    const wrapper = mountView()
    await flushPromises()
    await wrapper.get('[data-test="simulate-group-reorder"]').trigger('click')
    await flushPromises()

    expect(wrapper.findAll('[role="tab"]').map((tab) => tab.text().trim())).toEqual([
      'admin.accounts.allGroups',
      'last',
      'middle',
      'first',
      'admin.accounts.ungroupedTab'
    ])
    expect(JSON.parse(localStorage.getItem('account-group-tab-order') ?? '[]')).toEqual([9, 8, 7])
    wrapper.unmount()
  })

  it('uses measured tab widths and opens overflow groups from an arrow-only trigger', async () => {
    getAllGroups.mockResolvedValue([
      { id: 7, name: 'first', platform: 'openai' },
      { id: 8, name: 'middle', platform: 'openai' },
      { id: 9, name: 'last', platform: 'openai' }
    ])
    vi.spyOn(HTMLElement.prototype, 'clientWidth', 'get').mockImplementation(function () {
      return (this as HTMLElement).dataset.test === 'account-group-tabs' ? 400 : 0
    })
    vi.spyOn(HTMLElement.prototype, 'offsetWidth', 'get').mockImplementation(function () {
      const testID = (this as HTMLElement).dataset.test
      if (testID === 'account-group-tab-all') return 96
      if (testID === 'account-group-tab-ungrouped') return 80
      if (testID === 'account-group-tab-measurement') return 80
      if (testID === 'account-group-overflow-trigger') return 44
      return 0
    })

    const wrapper = mountView()
    await flushPromises()

    const visibleTabs = wrapper.get('[data-test="group-tabs-draggable"]')
    expect(visibleTabs.text()).toContain('first')
    expect(visibleTabs.text()).toContain('middle')
    expect(visibleTabs.text()).not.toContain('last')

    const trigger = wrapper.get('[data-test="account-group-overflow-trigger"]')
    expect(trigger.text()).toBe('')
    await trigger.trigger('click')

    expect(wrapper.get('[data-test="account-group-overflow-menu"]').text()).toContain('last')
    expect(wrapper.get('[data-test="account-group-tabs"]').classes()).not.toContain('overflow-hidden')
    wrapper.unmount()
  })

  it('persists overflow drag order without selecting a group and restores it after remount', async () => {
    getAllGroups.mockResolvedValue([
      { id: 7, name: 'first', platform: 'openai' },
      { id: 8, name: 'middle', platform: 'openai' },
      { id: 9, name: 'last', platform: 'openai' }
    ])
    vi.spyOn(HTMLElement.prototype, 'clientWidth', 'get').mockImplementation(function () {
      return (this as HTMLElement).dataset.test === 'account-group-tabs' ? 320 : 0
    })
    vi.spyOn(HTMLElement.prototype, 'offsetWidth', 'get').mockImplementation(function () {
      const testID = (this as HTMLElement).dataset.test
      if (testID === 'account-group-tab-all') return 96
      if (testID === 'account-group-tab-ungrouped') return 80
      if (testID === 'account-group-tab-measurement') return 80
      if (testID === 'account-group-overflow-trigger') return 44
      return 0
    })

    const wrapper = mountView()
    await flushPromises()
    await wrapper.get('[data-test="account-group-overflow-trigger"]').trigger('click')
    const menu = wrapper.get('[data-test="account-group-overflow-menu"]')
    await menu.get('[data-test="simulate-group-reorder"]').trigger('click')
    await menu.get('[role="tab"]').trigger('click')
    await flushPromises()

    expect(menu.findAll('[role="tab"]').map((tab) => tab.text())).toEqual(['last', 'middle'])
    expect(JSON.parse(localStorage.getItem('account-group-tab-order') ?? '[]')).toEqual([7, 9, 8])
    expect(wrapper.find('[data-test="account-group-overflow-menu"]').exists()).toBe(true)
    expect(listAccounts).toHaveBeenCalledTimes(1)
    wrapper.unmount()

    const restored = mountView()
    await flushPromises()
    await restored.get('[data-test="account-group-overflow-trigger"]').trigger('click')
    const restoredMenu = restored.get('[data-test="account-group-overflow-menu"]')
    expect(restoredMenu.findAll('[role="tab"]').map((tab) => tab.text())).toEqual(['last', 'middle'])
    await restoredMenu.get('[role="tab"]').trigger('click')
    await flushPromises()
    expect(restored.find('[data-test="account-group-overflow-menu"]').exists()).toBe(false)
    expect(listAccounts).toHaveBeenLastCalledWith(
      1, 20, expect.objectContaining({ group: '9' }), expect.anything()
    )
    restored.unmount()
  })

  it.each(['to-tabs', 'to-overflow'])('shares drag order across both lists: %s', async (direction) => {
    getAllGroups.mockResolvedValue([
      { id: 7, name: 'first', platform: 'openai' },
      { id: 8, name: 'middle', platform: 'openai' },
      { id: 9, name: 'last', platform: 'openai' }
    ])
    vi.spyOn(HTMLElement.prototype, 'clientWidth', 'get').mockImplementation(function () {
      return (this as HTMLElement).dataset.test === 'account-group-tabs' ? 400 : 0
    })
    vi.spyOn(HTMLElement.prototype, 'offsetWidth', 'get').mockImplementation(function () {
      const testID = (this as HTMLElement).dataset.test
      if (testID === 'account-group-tab-all') return 96
      if (testID === 'account-group-tab-ungrouped') return 80
      if (testID === 'account-group-tab-measurement') return 80
      if (testID === 'account-group-overflow-trigger') return 44
      return 0
    })

    const wrapper = mountView()
    await flushPromises()
    const tabs = wrapper.findAllComponents(VueDraggableStub)[0]
    // Starting a drag exposes the other drop target without an extra click.
    tabs.vm.$emit('start')
    await flushPromises()
    const menu = wrapper.findAllComponents(VueDraggableStub)[1]
    expect(tabs.attributes('group')).toBe('account-group-tabs')
    expect(menu.attributes('group')).toBe(tabs.attributes('group'))
    expect(menu.attributes('handle')).toBe(tabs.attributes('handle'))
    const visible = [...tabs.props('modelValue')]
    const overflow = [...menu.props('modelValue')]
    const source = direction === 'to-tabs' ? menu : tabs
    const target = direction === 'to-tabs' ? tabs : menu
    if (source === menu) source.vm.$emit('start')

    // Sortable emits the destination add before the source remove.
    target.vm.$emit('update:modelValue', direction === 'to-tabs'
      ? [overflow[0], ...visible]
      : [...overflow, visible[0]])
    await flushPromises()
    source.vm.$emit('update:modelValue', direction === 'to-tabs' ? [] : visible.slice(1))
    await flushPromises()
    // An empty source must remain mounted until the end event commits the order.
    expect(wrapper.findAllComponents(VueDraggableStub)).toHaveLength(2)
    source.vm.$emit('end')
    await flushPromises()

    const expected = direction === 'to-tabs' ? [9, 7, 8] : [8, 9, 7]
    expect(JSON.parse(localStorage.getItem('account-group-tab-order') ?? '[]')).toEqual(expected)
    expect(tabs.props('modelValue').map((tab: { value: string }) => Number(tab.value))).toEqual(expected.slice(0, 2))
    expect(menu.props('modelValue').map((tab: { value: string }) => Number(tab.value))).toEqual(expected.slice(2))
    expect(listAccounts).toHaveBeenCalledTimes(1)
    wrapper.unmount()

    const restored = mountView()
    await flushPromises()
    await restored.get('[data-test="account-group-overflow-trigger"]').trigger('click')
    expect(restored.findAllComponents(VueDraggableStub).flatMap((list) =>
      list.props('modelValue').map((tab: { value: string }) => Number(tab.value))
    )).toEqual(expected)
    restored.unmount()
  })

  it('maps group_ids through the group catalog for the table cell', async () => {
    const wrapper = mountView()
    await flushPromises()

    expect(wrapper.get('[data-test="account-groups"]').text()).toBe('codex')
    wrapper.unmount()
  })

  it('keeps the action menu open during internal scrolling but closes it on table scrolling', async () => {
    const wrapper = mountView(false)
    await flushPromises()

    const trigger = wrapper.findAll('button').find(button => button.text() === 'common.more')!
    await trigger.trigger('click')
    const menu = new DOMWrapper(document.body.querySelector('.action-menu-content')!)
    menu.element.dispatchEvent(new Event('scroll'))
    await flushPromises()
    expect(wrapper.findComponent(AccountActionMenu).props('show')).toBe(true)

    menu.get('button').element.dispatchEvent(new Event('scroll'))
    await flushPromises()
    expect(wrapper.findComponent(AccountActionMenu).props('show')).toBe(true)

    wrapper.getComponent(DataTableStub).element.dispatchEvent(new Event('scroll'))
    await flushPromises()
    expect(wrapper.findComponent(AccountActionMenu).props('show')).toBe(false)
    wrapper.unmount()
  })

  it('keeps lite=1 on automatic ETag refreshes', async () => {
    vi.useFakeTimers()
    vi.spyOn(document, 'hidden', 'get').mockReturnValue(false)
    localStorage.setItem('account-auto-refresh', JSON.stringify({ enabled: true, interval_seconds: 5 }))
    const wrapper = mountView()
    await flushPromises()

    await vi.advanceTimersByTimeAsync(6000)
    await flushPromises()

    expect(listWithEtag).toHaveBeenCalledWith(
      1,
      20,
      expect.objectContaining({ lite: '1' }),
      expect.objectContaining({ etag: null })
    )
    wrapper.unmount()
  })

  it('loads the full account by id before opening edit, test, and stats actions', async () => {
    const wrapper = mountView()
    await flushPromises()

    const editButton = wrapper.findAll('button').find(button => button.text().includes('common.edit'))
    expect(editButton).toBeTruthy()
    await editButton!.trigger('click')
    await flushPromises()
    expect(getById).toHaveBeenCalledWith(42)
    expect(wrapper.get('[data-test="edit-account"]').text()).toBe('compact row')

    const menu = wrapper.findComponent(AccountActionMenu)
    menu.vm.$emit('test', listRow)
    await flushPromises()
    expect(getById).toHaveBeenCalledTimes(2)
    expect(wrapper.get('[data-test="test-account"]').text()).toBe('compact row')

    menu.vm.$emit('stats', listRow)
    await flushPromises()
    expect(getById).toHaveBeenCalledTimes(3)
    expect(wrapper.get('[data-test="stats-account"]').text()).toBe('compact row')
    wrapper.unmount()
  })

  it('shows an error and keeps the modal closed when detail loading fails', async () => {
    const consoleError = vi.spyOn(console, 'error').mockImplementation(() => {})
    getById.mockRejectedValueOnce(new Error('detail failed'))
    const wrapper = mountView()
    await flushPromises()

    const editButton = wrapper.findAll('button').find(button => button.text().includes('common.edit'))
    await editButton!.trigger('click')
    await flushPromises()

    expect(showError).toHaveBeenCalledWith('detail failed')
    expect(wrapper.get('[data-test="edit-account"]').text()).toBe('')
    consoleError.mockRestore()
    wrapper.unmount()
  })
})
