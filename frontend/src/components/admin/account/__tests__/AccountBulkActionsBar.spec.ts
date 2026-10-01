import { describe, expect, it, vi } from 'vitest'
import { mount } from '@vue/test-utils'

import AccountBulkActionsBar from '../AccountBulkActionsBar.vue'

vi.mock('vue-i18n', () => ({
  useI18n: () => ({
    t: (key: string) => key
  })
}))

describe('AccountBulkActionsBar', () => {
  it('shows model sync only for selected accounts and disables duplicate actions', async () => {
    const wrapper = mount(AccountBulkActionsBar, { props: { selectedIds: [], totalResults: 2, selectingAll: false, allResultsSelected: false } })
    const findSync = () => wrapper.findAll('button').find(button => button.attributes('title') === 'admin.accounts.batchModelSync.hint')
    expect(findSync()).toBeUndefined()
    await wrapper.setProps({ selectedIds: [1, 2] })
    await findSync()!.trigger('click')
    expect(wrapper.emitted('sync-upstream-models')).toHaveLength(1)
    await wrapper.setProps({ syncingModels: true })
    expect(findSync()!.attributes('disabled')).toBeDefined()
    await findSync()!.trigger('click')
    expect(wrapper.emitted('sync-upstream-models')).toHaveLength(1)
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
