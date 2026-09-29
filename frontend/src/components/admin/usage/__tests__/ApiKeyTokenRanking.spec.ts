import { describe, expect, it, vi, beforeEach } from 'vitest'
import { mount, flushPromises } from '@vue/test-utils'

import ApiKeyTokenRanking from '../ApiKeyTokenRanking.vue'

const getApiKeysRanking = vi.fn()

vi.mock('@/api/admin/dashboard', () => ({
  getApiKeysRanking: (...args: unknown[]) => getApiKeysRanking(...args),
}))

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return {
    ...actual,
    useI18n: () => ({
      t: (key: string) => key,
    }),
  }
})

const item = (id: number, tokens: number, overrides: Record<string, unknown> = {}) => ({
  api_key_id: id,
  key_name: `key-${id}`,
  key_deleted: false,
  user_id: id,
  email: `u${id}@test.com`,
  username: `u${id}`,
  requests: 1,
  input_tokens: tokens,
  output_tokens: 0,
  cache_tokens: 0,
  total_tokens: tokens,
  cost: 0.6,
  actual_cost: 0.5,
  ...overrides,
})

const response = (items: unknown[]) => ({
  ranking: items,
  total_actual_cost: 1,
  total_requests: 2,
  total_tokens: 150,
  start_date: '2026-07-01',
  end_date: '2026-07-08',
})

const mountRanking = (props: Record<string, unknown> = {}) =>
  mount(ApiKeyTokenRanking, {
    props: {
      startDate: '2026-07-01',
      endDate: '2026-07-08',
      ...props,
    },
    global: { stubs: { Select: true, LoadingSpinner: true } },
  })

describe('ApiKeyTokenRanking', () => {
  beforeEach(() => {
    getApiKeysRanking.mockReset()
    getApiKeysRanking.mockResolvedValue(response([item(1, 100), item(2, 50, { key_deleted: true })]))
  })

  it('loads on mount and emits select-key with id + name on row click', async () => {
    const wrapper = mountRanking({ userId: 7 })
    await flushPromises()

    expect(getApiKeysRanking).toHaveBeenCalledWith({
      start_date: '2026-07-01',
      end_date: '2026-07-08',
      sort_by: 'total_tokens',
      limit: 50,
      user_id: 7,
    })

    const rows = wrapper.findAll('tbody tr')
    expect(rows).toHaveLength(2)
    expect(rows[0].text()).toContain('key-1')
    expect(rows[0].text()).toContain('u1@test.com')
    expect(rows[1].text()).toContain('admin.dashboard.apiKeyDeletedBadge')

    await rows[0].trigger('click')
    expect(wrapper.emitted('select-key')![0]).toEqual([1, 'key-1'])
  })

  it('refetches with the clicked sort column', async () => {
    const wrapper = mountRanking()
    await flushPromises()
    expect(getApiKeysRanking).toHaveBeenCalledTimes(1)

    const costHeader = wrapper
      .findAll('thead th')
      .find((th) => th.text().includes('admin.usage.keyRanking.columns.cost'))
    expect(costHeader).toBeTruthy()
    await costHeader!.trigger('click')
    await flushPromises()

    expect(getApiKeysRanking).toHaveBeenCalledTimes(2)
    expect(getApiKeysRanking).toHaveBeenLastCalledWith(
      expect.objectContaining({ sort_by: 'actual_cost' })
    )
  })

  it('reloads when the shared date range or user filter changes', async () => {
    const wrapper = mountRanking()
    await flushPromises()
    expect(getApiKeysRanking).toHaveBeenCalledTimes(1)

    await wrapper.setProps({ userId: 9 })
    await flushPromises()

    expect(getApiKeysRanking).toHaveBeenCalledTimes(2)
    expect(getApiKeysRanking).toHaveBeenLastCalledWith(expect.objectContaining({ user_id: 9 }))
  })
})
