import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount, RouterLinkStub } from '@vue/test-utils'

import SubscriptionsView from '../SubscriptionsView.vue'

const { getMySubscriptions, showError } = vi.hoisted(() => ({
  getMySubscriptions: vi.fn(),
  showError: vi.fn(),
}))

vi.mock('@/api/subscriptions', () => ({ default: { getMySubscriptions } }))

vi.mock('@/stores/app', () => ({
  useAppStore: () => ({ showError, cachedPublicSettings: {} }),
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
