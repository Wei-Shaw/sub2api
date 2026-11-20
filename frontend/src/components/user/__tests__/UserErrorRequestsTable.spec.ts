import { afterEach, describe, expect, it, vi } from 'vitest'
import { enableAutoUnmount, mount } from '@vue/test-utils'
import UserErrorRequestsTable from '../UserErrorRequestsTable.vue'
import type { UserErrorRequest } from '@/types'

vi.mock('vue-i18n', async (importOriginal) => ({
  ...await importOriginal<typeof import('vue-i18n')>(),
  useI18n: () => ({ t: (key: string) => key }),
}))
enableAutoUnmount(afterEach)

describe('UserErrorRequestsTable request parameters', () => {
  it('shows explicit effort and tier and leaves missing values empty', () => {
    const wrapper = mount(UserErrorRequestsTable, {
      props: {
        rows: [
          { id: 1, reasoning_effort: 'Not-Allowed', service_tier: 'default' },
          { id: 2 },
        ] as UserErrorRequest[],
        total: 2,
        loading: false,
        page: 1,
        pageSize: 20,
        visibleColumnKeys: ['reasoning_effort', 'service_tier'],
      },
      global: { stubs: { UserErrorDetailModal: true, Pagination: true, IpGeoBatchToolbar: true } },
    })
    expect(wrapper.findAll('thead th').map((cell) => cell.text())).toEqual(['usage.reasoningEffort', 'service_tier'])
    expect(wrapper.findAll('tbody tr').map((row) => row.findAll('td').map((cell) => cell.text()))).toEqual([
      ['Not-Allowed', 'default'],
      ['-', '-'],
    ])
  })
})
