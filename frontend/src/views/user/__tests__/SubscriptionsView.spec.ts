import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount, RouterLinkStub } from '@vue/test-utils'

import SubscriptionsView from '../SubscriptionsView.vue'

const { getMySubscriptions, showError, appSettings } = vi.hoisted(() => ({
  getMySubscriptions: vi.fn(),
  showError: vi.fn(),
  appSettings: {} as Record<string, unknown>,
}))

vi.mock('@/api/subscriptions', () => ({ default: { getMySubscriptions } }))

vi.mock('@/stores/app', () => ({
  useAppStore: () => ({ showError, cachedPublicSettings: appSettings }),
}))

vi.mock('vue-router', () => ({ useRouter: () => ({ push: vi.fn() }) }))

vi.mock('vue-i18n', async (importOriginal) => {
  const actual = await importOriginal<typeof import('vue-i18n')>()
  return { ...actual, useI18n: () => ({ t: (key: string) => key }) }
})

const mountView = async () => {
  const wrapper = mount(SubscriptionsView, {
    global: {
      stubs: { AppLayout: { template: '<main><slot /></main>' }, Icon: true, RouterLink: RouterLinkStub },
    },
  })
  await flushPromises()
  return wrapper
}

describe('user SubscriptionsView', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    vi.spyOn(console, 'error').mockImplementation(() => {})
    getMySubscriptions.mockResolvedValue([])
    delete appSettings.payment_enabled
  })

  it('points to the purchase page when there is no subscription and payment is on', async () => {
    const wrapper = await mountView()

    expect(wrapper.text()).toContain('userSubscriptions.noActiveSubscriptionsPurchaseDesc')
    expect(wrapper.text()).not.toContain('userSubscriptions.noActiveSubscriptionsDesc')
    expect(wrapper.getComponent(RouterLinkStub).props('to')).toEqual({ path: '/purchase', query: { tab: 'subscription' } })
  })

  it('keeps the contact-admin text without a link when payment is off', async () => {
    appSettings.payment_enabled = false
    const wrapper = await mountView()

    expect(wrapper.text()).toContain('userSubscriptions.noActiveSubscriptionsDesc')
    expect(wrapper.findComponent(RouterLinkStub).exists()).toBe(false)
  })

  it('reports a load failure with a retry instead of claiming there are no subscriptions', async () => {
    getMySubscriptions.mockRejectedValueOnce({ status: 0, message: 'Network error' })
    const wrapper = await mountView()

    expect(wrapper.text()).toContain('userSubscriptions.failedToLoad')
    expect(wrapper.text()).not.toContain('userSubscriptions.noActiveSubscriptions')
    expect(showError).toHaveBeenCalledWith('userSubscriptions.failedToLoad')

    await wrapper.findAll('button').find((button) => button.text() === 'common.refresh')!.trigger('click')
    await flushPromises()
    expect(getMySubscriptions).toHaveBeenCalledTimes(2)
    expect(wrapper.text()).not.toContain('userSubscriptions.failedToLoad')
    expect(wrapper.text()).toContain('userSubscriptions.noActiveSubscriptions')
  })
})
