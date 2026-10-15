import { describe, expect, it, vi } from 'vitest'
import { mount } from '@vue/test-utils'

import AccountBulkActionsBar from '../AccountBulkActionsBar.vue'

vi.mock('vue-i18n', () => ({
  useI18n: () => ({
    t: (key: string) => key
  })
}))

describe('AccountBulkActionsBar', () => {
  it('offers only the selected scope when two of three accounts are checked', async () => {
    const wrapper = mount(AccountBulkActionsBar, {
      props: { selectedIds: [1, 2], totalResults: 3, selectingAll: false, allResultsSelected: false }
    })
    const editButtons = wrapper.findAll('button.btn-primary')

    expect(editButtons).toHaveLength(1)
    await editButtons[0].trigger('click')
    expect(wrapper.emitted('edit-selected')).toHaveLength(1)
    expect(wrapper.emitted('edit-filtered')).toBeUndefined()
  })

  it('allows explicit filtered editing only when matching accounts exist', async () => {
    const wrapper = mount(AccountBulkActionsBar, {
      props: { selectedIds: [], totalResults: 0, selectingAll: false, allResultsSelected: false }
    })
    const button = wrapper.get('button.btn-primary')
    expect(button.attributes('disabled')).toBeDefined()
    await button.trigger('click')
    expect(wrapper.emitted('edit-filtered')).toBeUndefined()

    await wrapper.setProps({ totalResults: 3 })
    expect(button.text()).toContain('admin.accounts.bulkActions.editFiltered')
    await button.trigger('click')
    expect(wrapper.emitted('edit-filtered')).toHaveLength(1)
  })

  it('allows selecting all results before any row is selected', async () => {
    const wrapper = mount(AccountBulkActionsBar, {
      props: {
        selectedIds: [],
        totalResults: 45,
        selectingAll: false,
        allResultsSelected: false
      }
    })

    const button = wrapper.findAll('button').find(item =>
      item.text().includes('admin.accounts.bulkActions.selectAllResults')
    )

    expect(button).toBeDefined()
    await button!.trigger('click')
    expect(wrapper.emitted('select-all-results')).toHaveLength(1)
  })

  it('preserves the upstream billing probe action from v0.1.166', async () => {
    const wrapper = mount(AccountBulkActionsBar, {
      props: {
        selectedIds: [1],
        totalResults: 45,
        selectingAll: false,
        allResultsSelected: false
      }
    })

    const button = wrapper.findAll('button').find(item =>
      item.text().includes('admin.accounts.bulkActions.probeUpstreamBilling')
    )

    expect(button).toBeDefined()
    await button!.trigger('click')
    expect(wrapper.emitted('probe-upstream-billing')).toHaveLength(1)
  })
})
