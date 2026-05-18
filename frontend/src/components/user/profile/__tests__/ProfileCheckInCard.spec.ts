import { flushPromises, mount } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import ProfileCheckInCard from '@/components/user/profile/ProfileCheckInCard.vue'
import { userAPI, type DailyCheckInStatus } from '@/api/user'

const showSuccess = vi.fn()
const showError = vi.fn()
const refreshUser = vi.fn()

vi.mock('@/api/user', () => ({
  userAPI: {
    getDailyCheckInStatus: vi.fn(),
    applyDailyCheckIn: vi.fn(),
  },
}))

vi.mock('@/stores/app', () => ({
  useAppStore: () => ({
    showSuccess,
    showError,
  }),
}))

vi.mock('@/stores/auth', () => ({
  useAuthStore: () => ({
    refreshUser,
  }),
}))

vi.mock('vue-i18n', async (importOriginal) => {
  const actual = await importOriginal<typeof import('vue-i18n')>()
  return {
    ...actual,
    useI18n: () => ({
      t: (key: string, params?: Record<string, unknown>) => {
        if (key === 'profile.checkIn.days') {
          return `${params?.count ?? 0} days`
        }
        if (key === 'profile.checkIn.success') {
          return `Reward credited: ${params?.amount ?? ''}`
        }
        if (key === 'common.processing') {
          return 'Processing'
        }
        return key
      },
    }),
  }
})

function status(overrides: Partial<DailyCheckInStatus> = {}): DailyCheckInStatus {
  return {
    enabled: true,
    reward_amount: 0.75,
    check_in_days: 3,
    checked_in_today: false,
    last_check_in_at: null,
    ...overrides,
  }
}

function mountCard() {
  return mount(ProfileCheckInCard, {
    global: {
      stubs: {
        Icon: true,
      },
    },
  })
}

function mountCardWithDisabledState() {
  return mount(ProfileCheckInCard, {
    props: {
      showDisabled: true,
    },
    global: {
      stubs: {
        Icon: true,
      },
    },
  })
}

describe('ProfileCheckInCard', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    refreshUser.mockResolvedValue(undefined)
  })

  it('stays hidden when daily check-in is disabled', async () => {
    vi.mocked(userAPI.getDailyCheckInStatus).mockResolvedValue(status({ enabled: false }))

    const wrapper = mountCard()
    await flushPromises()

    expect(wrapper.find('[data-testid="profile-checkin-card"]').exists()).toBe(false)
  })

  it('shows a disabled state when requested', async () => {
    vi.mocked(userAPI.getDailyCheckInStatus).mockResolvedValue(status({ enabled: false }))

    const wrapper = mountCardWithDisabledState()
    await flushPromises()

    expect(wrapper.get('[data-testid="profile-checkin-card"]').text()).toContain('每日签到未开启')
  })

  it('renders an available check-in action', async () => {
    vi.mocked(userAPI.getDailyCheckInStatus).mockResolvedValue(status())

    const wrapper = mountCard()
    await flushPromises()

    expect(wrapper.get('[data-testid="profile-checkin-card"]').text()).toContain('$0.75')
    expect(wrapper.get('button').text()).toContain('立即签到')
    expect(wrapper.get('button').attributes('disabled')).toBeUndefined()
  })

  it('claims the reward and refreshes the profile', async () => {
    vi.mocked(userAPI.getDailyCheckInStatus).mockResolvedValue(status())
    vi.mocked(userAPI.applyDailyCheckIn).mockResolvedValue(status({
      check_in_days: 4,
      checked_in_today: true,
      last_check_in_at: '2026-05-12T10:00:00Z',
    }))

    const wrapper = mountCard()
    await flushPromises()
    await wrapper.get('button').trigger('click')
    await flushPromises()

    expect(userAPI.applyDailyCheckIn).toHaveBeenCalledTimes(1)
    expect(refreshUser).toHaveBeenCalledTimes(1)
    expect(showSuccess).toHaveBeenCalledWith('Reward credited: $0.75')
    expect(wrapper.emitted('checkedIn')).toHaveLength(1)
    expect(wrapper.get('button').attributes('disabled')).toBeDefined()
    expect(wrapper.text()).toContain('今日已签到')
  })

  it('shows API errors when check-in fails', async () => {
    vi.mocked(userAPI.getDailyCheckInStatus).mockResolvedValue(status())
    vi.mocked(userAPI.applyDailyCheckIn).mockRejectedValue({
      response: {
        data: {
          message: 'already checked',
        },
      },
    })

    const wrapper = mountCard()
    await flushPromises()
    await wrapper.get('button').trigger('click')
    await flushPromises()

    expect(showError).toHaveBeenCalledWith('already checked')
    expect(refreshUser).not.toHaveBeenCalled()
  })
})
