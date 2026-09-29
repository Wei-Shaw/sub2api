import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount, RouterLinkStub } from '@vue/test-utils'
import RedeemView from '../RedeemView.vue'

const {
  f211Redeem,
  f211GetHistory,
  f211RefreshUser,
  f211FetchActiveSubscriptions,
  f211FetchPublicSettings,
  f211ShowError,
  f211ShowWarning,
  f211ShowSuccess
} = vi.hoisted(() => ({
  f211Redeem: vi.fn(),
  f211GetHistory: vi.fn(),
  f211RefreshUser: vi.fn(),
  f211FetchActiveSubscriptions: vi.fn(),
  f211FetchPublicSettings: vi.fn(),
  f211ShowError: vi.fn(),
  f211ShowWarning: vi.fn(),
  f211ShowSuccess: vi.fn()
}))

vi.mock('@/api', () => ({
  redeemAPI: { redeem: f211Redeem, getHistory: f211GetHistory }
}))
vi.mock('@/stores/auth', () => ({
  useAuthStore: () => ({ user: { balance: 10, concurrency: 2 }, refreshUser: f211RefreshUser })
}))
vi.mock('@/stores/subscriptions', () => ({
  useSubscriptionStore: () => ({ fetchActiveSubscriptions: f211FetchActiveSubscriptions })
}))
vi.mock('@/stores/app', () => ({
  useAppStore: () => ({
    showError: f211ShowError,
    showWarning: f211ShowWarning,
    showSuccess: f211ShowSuccess,
    fetchPublicSettings: f211FetchPublicSettings
  })
}))
vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return { ...actual, useI18n: () => ({ t: (key: string) => key }) }
})

function f211MountRedeem() {
  return mount(RedeemView, {
    global: {
      stubs: { AppLayout: { template: '<div><slot /></div>' }, Icon: true, RouterLink: RouterLinkStub }
    }
  })
}

describe('RedeemView history rows (F2-11)', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    f211FetchPublicSettings.mockResolvedValue({})
    vi.spyOn(console, 'error').mockImplementation(() => {})
  })

  afterEach(() => {
    vi.restoreAllMocks()
  })

  it('labels invitation rows and shows no bogus request count', async () => {
    f211GetHistory.mockResolvedValue({
      total: 1,
      items: [{
        id: 1,
        code: 'INVITE-CODE-1',
        type: 'invitation',
        value: 0,
        status: 'used',
        used_at: '2026-03-08T00:00:00Z',
        created_at: '2026-03-01T00:00:00Z'
      }]
    })
    const wrapper = f211MountRedeem()
    await flushPromises()

    const text = wrapper.text()
    expect(text).toContain('redeem.invitationCodeUsed')
    expect(text).not.toContain('common.unknown')
    expect(text).not.toContain('+0 redeem.requests')
    wrapper.unmount()
  })

  it('renders negative balance amounts as -$5.00', async () => {
    f211GetHistory.mockResolvedValue({
      total: 2,
      items: [
        {
          id: 2,
          code: 'ADMINADJ',
          type: 'admin_balance',
          value: -5,
          status: 'used',
          used_at: '2026-03-08T00:00:00Z',
          created_at: '2026-03-01T00:00:00Z',
          notes: 'refund'
        },
        {
          id: 3,
          code: 'ADMINADJ2',
          type: 'admin_balance',
          value: 7.5,
          status: 'used',
          used_at: '2026-03-08T00:00:00Z',
          created_at: '2026-03-01T00:00:00Z'
        }
      ]
    })
    const wrapper = f211MountRedeem()
    await flushPromises()

    const text = wrapper.text()
    expect(text).toContain('-$5.00')
    expect(text).not.toContain('$-5.00')
    expect(text).toContain('+$7.50')
    wrapper.unmount()
  })
})
