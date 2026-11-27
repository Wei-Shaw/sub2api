import { mount } from '@vue/test-utils'
import { describe, expect, it, vi } from 'vitest'
import AccountTodayStatsCell from '../AccountTodayStatsCell.vue'
import type { WindowStats } from '@/types'

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return {
    ...actual,
    useI18n: () => ({
      t: (key: string) => key
    })
  }
})

const baseStats: WindowStats = {
  requests: 10,
  tokens: 1000,
  cost: 1,
  standard_cost: 1,
  user_cost: 1
}

const mountCell = (stats: WindowStats) => mount(AccountTodayStatsCell, { props: { stats } })

describe('AccountTodayStatsCell upstream errors', () => {
  it('highlights a non-zero upstream error count', () => {
    const row = mountCell({ ...baseStats, upstream_errors: 12 }).get('[data-testid="today-upstream-errors"]')
    expect(row.text()).toContain('admin.accounts.stats.upstreamErrors')
    expect(row.text()).toContain('12')
    expect(row.find('.text-red-600').exists()).toBe(true)
  })

  it('shows zero without highlighting', () => {
    const row = mountCell({ ...baseStats, upstream_errors: 0 }).get('[data-testid="today-upstream-errors"]')
    expect(row.text()).toContain('0')
    expect(row.find('.text-red-600').exists()).toBe(false)
  })

  it('hides the row when the backend omits the count', () => {
    expect(mountCell(baseStats).find('[data-testid="today-upstream-errors"]').exists()).toBe(false)
    expect(
      mountCell({ ...baseStats, upstream_errors: null }).find('[data-testid="today-upstream-errors"]').exists()
    ).toBe(false)
  })
})
