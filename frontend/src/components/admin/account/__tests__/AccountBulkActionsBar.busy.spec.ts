import { describe, expect, it, vi } from 'vitest'
import { mount } from '@vue/test-utils'

import AccountBulkActionsBar from '../AccountBulkActionsBar.vue'

vi.mock('vue-i18n', () => ({
  useI18n: () => ({
    t: (key: string, params?: Record<string, unknown>) => (params ? `${key}:${JSON.stringify(params)}` : key)
  })
}))

const mountBar = (busy?: boolean) => mount(AccountBulkActionsBar, {
  props: {
    selectedIds: [1, 2, 3],
    totalResults: 3,
    selectingAll: false,
    allResultsSelected: false,
    busy
  }
})

describe('AccountBulkActionsBar busy state', () => {
  it('disables every bulk action and shows progress while a bulk action runs', async () => {
    const wrapper = mountBar(true)

    expect(wrapper.get('[data-testid="bulk-busy"]').text()).toContain(
      'admin.accounts.bulkActions.processing:{"count":3}'
    )
    for (const key of ['delete', 'resetStatus', 'refreshToken', 'probeUpstreamBilling', 'enableScheduling', 'disableScheduling', 'edit']) {
      const button = wrapper.findAll('button').find(item => item.text() === `admin.accounts.bulkActions.${key}`)
      expect(button?.attributes('disabled'), key).toBeDefined()
    }

    await wrapper.findAll('button').find(item => item.text() === 'admin.accounts.bulkActions.refreshToken')!.trigger('click')
    expect(wrapper.emitted('refresh-token')).toBeUndefined()
  })

  it('keeps actions enabled when idle', () => {
    const wrapper = mountBar()

    expect(wrapper.find('[data-testid="bulk-busy"]').exists()).toBe(false)
    const button = wrapper.findAll('button').find(item => item.text() === 'admin.accounts.bulkActions.delete')
    expect(button?.attributes('disabled')).toBeUndefined()
  })
})
