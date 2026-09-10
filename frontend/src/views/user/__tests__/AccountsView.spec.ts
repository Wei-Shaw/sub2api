import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import { defineComponent } from 'vue'

import AccountsView from '../AccountsView.vue'

const { listAccounts, listGroups } = vi.hoisted(() => ({
  listAccounts: vi.fn(),
  listGroups: vi.fn()
}))

vi.mock('@/api/accounts', () => ({
  accountsAPI: {
    list: listAccounts,
    listGroups
  }
}))

vi.mock('@/stores/app', () => ({
  useAppStore: () => ({ showError: vi.fn() })
}))

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return { ...actual, useI18n: () => ({ t: (key: string) => key }) }
})

const DataTableStub = defineComponent({
  props: { columns: { type: Array, default: () => [] }, data: { type: Array, default: () => [] } },
  template: `
    <div data-test="account-table" :data-columns="columns.map(column => column.key).join(',')">
      <div v-for="row in data" :key="row.id"><slot name="cell-actions" :row="row" /></div>
    </div>
  `
})

const AccountActionMenuStub = defineComponent({
  props: { show: Boolean, account: { type: Object, default: null }, readOnly: Boolean },
  emits: ['close', 'test', 'stats'],
  template: '<div data-test="account-action-menu" :data-read-only="String(readOnly)" />'
})

function mountView() {
  return mount(AccountsView, {
    attachTo: document.body,
    global: {
      stubs: {
        AppLayout: { template: '<div><slot /></div>' },
        TablePageLayout: { template: '<div><slot name="filters" /><slot name="table" /><slot name="pagination" /></div>' },
        DataTable: DataTableStub,
        Pagination: true,
        SearchInput: true,
        Select: true,
        PlatformTypeBadge: true,
        Icon: true,
        VueDraggable: { template: '<div><slot /></div>' },
        AccountActionMenu: AccountActionMenuStub,
        AccountTestModal: true,
        AccountStatsModal: true
      }
    }
  })
}

const account = {
  id: 42,
  name: 'visible account',
  platform: 'openai',
  type: 'oauth',
  concurrency: 2,
  priority: 50,
  status: 'active',
  schedulable: true,
  last_used_at: null,
  expires_at: null,
  created_at: '2026-09-11T00:00:00Z',
  rate_limit_reset_at: null,
  overload_until: null,
  temp_unschedulable_until: null,
  groups: [],
  group_ids: []
}

describe('user AccountsView', () => {
  beforeEach(() => {
    localStorage.clear()
    listAccounts.mockReset().mockResolvedValue({ items: [account], total: 1, page: 1, page_size: 20, pages: 1 })
    listGroups.mockReset().mockResolvedValue([])
  })

  it('keeps the operation column without edit and delete buttons', async () => {
    const wrapper = mountView()
    await flushPromises()

    expect(wrapper.get('[data-test="account-table"]').attributes('data-columns')).toContain('actions')
    expect(wrapper.text()).not.toContain('common.edit')
    expect(wrapper.text()).not.toContain('common.delete')
    expect(wrapper.text()).toContain('common.more')
    expect(wrapper.get('[data-test="account-action-menu"]').attributes('data-read-only')).toBe('true')
    wrapper.unmount()
  })

  it('persists column visibility settings in browser storage', async () => {
    const wrapper = mountView()
    await flushPromises()

    const moreButton = wrapper.findAll('button').find((button) => button.text().includes('visibleAccounts.moreActions'))
    expect(moreButton).toBeTruthy()
    await moreButton!.trigger('click')
    const idColumnButton = wrapper.findAll('button').find((button) => button.text().includes('visibleAccounts.columns.id'))
    expect(idColumnButton).toBeTruthy()
    await idColumnButton!.trigger('click')

    expect(JSON.parse(localStorage.getItem('visible-account-hidden-columns') ?? '[]')).toContain('id')
    expect(wrapper.get('[data-test="account-table"]').attributes('data-columns')).not.toContain('id')
    wrapper.unmount()
  })
})
