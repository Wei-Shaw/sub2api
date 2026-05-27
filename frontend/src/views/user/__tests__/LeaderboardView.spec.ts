import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'

import LeaderboardView from '../LeaderboardView.vue'

const { getDailyTokenLeaderboard } = vi.hoisted(() => ({
  getDailyTokenLeaderboard: vi.fn(),
}))

const messages: Record<string, string> = {
  'leaderboard.title': 'Daily Token Leaderboard',
  'leaderboard.subtitle': 'Top 5 token users today',
  'leaderboard.rank': 'Rank',
  'leaderboard.user': 'User',
  'leaderboard.tokensToday': 'Tokens Today',
  'leaderboard.empty': 'No token usage today',
  'leaderboard.failedToLoad': 'Failed to load leaderboard',
  'leaderboard.refresh': 'Refresh',
  'leaderboard.refreshing': 'Refreshing...',
}

vi.mock('@/api/usage', () => ({
  usageAPI: {
    getDailyTokenLeaderboard,
  },
}))

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return {
    ...actual,
    useI18n: () => ({
      t: (key: string) => messages[key] ?? key,
    }),
  }
})

const AppLayoutStub = { template: '<div><slot /></div>' }
const LoadingSpinnerStub = { template: '<div data-testid="loading-spinner" />' }

function mountView() {
  return mount(LeaderboardView, {
    global: {
      stubs: {
        AppLayout: AppLayoutStub,
        LoadingSpinner: LoadingSpinnerStub,
      },
    },
  })
}

describe('LeaderboardView', () => {
  beforeEach(() => {
    getDailyTokenLeaderboard.mockReset()
    vi.spyOn(console, 'error').mockImplementation(() => {})
  })

  it('renders the top 5 rows with formatted token counts', async () => {
    getDailyTokenLeaderboard.mockResolvedValue({
      items: [
        { rank: 1, display_name: 'ali***', total_tokens: 1234567 },
        { rank: 2, display_name: 'bo***', total_tokens: 98765 },
      ],
      start_date: '2026-05-27',
      end_date: '2026-05-27',
      limit: 5,
      generated_at: '2026-05-27T12:00:00+08:00',
    })

    const wrapper = mountView()
    await flushPromises()

    expect(wrapper.text()).toContain('Daily Token Leaderboard')
    expect(wrapper.text()).toContain('#1')
    expect(wrapper.text()).toContain('ali***')
    expect(wrapper.text()).toContain('1,234,567')
    expect(wrapper.text()).toContain('#2')
    expect(wrapper.text()).toContain('bo***')
    expect(wrapper.text()).toContain('98,765')
  })

  it('renders an empty state when there are no leaderboard rows', async () => {
    getDailyTokenLeaderboard.mockResolvedValue({
      items: [],
      start_date: '2026-05-27',
      end_date: '2026-05-27',
      limit: 5,
      generated_at: '2026-05-27T12:00:00+08:00',
    })

    const wrapper = mountView()
    await flushPromises()

    expect(wrapper.text()).toContain('No token usage today')
  })

  it('renders an error state and can retry manually', async () => {
    getDailyTokenLeaderboard.mockRejectedValueOnce(new Error('network'))
    getDailyTokenLeaderboard.mockResolvedValueOnce({
      items: [{ rank: 1, display_name: 'zoe***', total_tokens: 42 }],
      start_date: '2026-05-27',
      end_date: '2026-05-27',
      limit: 5,
      generated_at: '2026-05-27T12:00:00+08:00',
    })

    const wrapper = mountView()
    await flushPromises()

    expect(wrapper.text()).toContain('Failed to load leaderboard')

    await wrapper.get('button').trigger('click')
    await flushPromises()

    expect(getDailyTokenLeaderboard).toHaveBeenCalledTimes(2)
    expect(wrapper.text()).toContain('zoe***')
  })
})
