import { describe, expect, it, vi } from 'vitest'
import { mount } from '@vue/test-utils'

import AccountBulkActionsBar from '../AccountBulkActionsBar.vue'

vi.mock('vue-i18n', () => ({
  useI18n: () => ({
    t: (key: string) => key
  })
}))

const mountBar = (selectedIds: number[] = []) => mount(AccountBulkActionsBar, {
  props: {
    selectedIds,
    totalResults: 45,
    selectingAll: false,
    allResultsSelected: false
  },
  global: {
    stubs: { Icon: true }
  }
})

describe('AccountBulkActionsBar', () => {
  it('allows selecting all results before any row is selected', async () => {
    const wrapper = mountBar()
    const button = wrapper.findAll('button').find(item =>
      item.text().includes('admin.accounts.bulkActions.selectAllResults')
    )

    expect(button).toBeDefined()
    await button!.trigger('click')
    expect(wrapper.emitted('select-all-results')).toHaveLength(1)
  })

  it('preserves the upstream billing probe action from v0.1.166', async () => {
    const wrapper = mountBar([1])
    const button = wrapper.findAll('button').find(item =>
      item.text().includes('admin.accounts.bulkActions.probeUpstreamBilling')
    )

    expect(button).toBeDefined()
    await button!.trigger('click')
    expect(wrapper.emitted('probe-upstream-billing')).toHaveLength(1)
  })

  it('keeps the proxy pool action visible but disabled before accounts are selected', () => {
    const wrapper = mountBar()
    const button = wrapper.get('[data-test="bind-proxy-pool"]')

    expect(button.attributes('disabled')).toBeDefined()
    expect(button.attributes('title')).toBe('admin.accounts.bulkActions.selectBeforeBind')
  })

  it('enables the proxy pool action and emits after accounts are selected', async () => {
    const wrapper = mountBar([101, 102])
    const button = wrapper.get('[data-test="bind-proxy-pool"]')

    expect(button.attributes('disabled')).toBeUndefined()
    await button.trigger('click')
    expect(wrapper.emitted('bind-proxy-pool')).toHaveLength(1)
  })
})
