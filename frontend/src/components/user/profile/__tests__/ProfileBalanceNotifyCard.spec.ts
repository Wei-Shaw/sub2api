import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { enableAutoUnmount, flushPromises, mount, type VueWrapper } from '@vue/test-utils'
import ProfileBalanceNotifyCard from '../ProfileBalanceNotifyCard.vue'

const { sendNotifyEmailCode, verifyNotifyEmail, getProfile } = vi.hoisted(() => ({
  sendNotifyEmailCode: vi.fn(),
  verifyNotifyEmail: vi.fn(),
  getProfile: vi.fn()
}))

vi.mock('@/api', () => ({
  userAPI: { sendNotifyEmailCode, verifyNotifyEmail, getProfile }
}))
vi.mock('@/stores/auth', () => ({ useAuthStore: () => ({ user: null }) }))
vi.mock('@/stores/app', () => ({
  useAppStore: () => ({ showSuccess: vi.fn(), showError: vi.fn() })
}))
vi.mock('vue-i18n', () => ({ useI18n: () => ({ t: (key: string) => key }) }))

enableAutoUnmount(afterEach)

const deferred = () => {
  let resolve!: () => void
  const promise = new Promise<void>((done) => { resolve = done })
  return { promise, resolve }
}

const pendingRows = (wrapper: VueWrapper) => wrapper.findAll('.bg-yellow-50')
const pendingEmails = (wrapper: VueWrapper) => pendingRows(wrapper).map(row => row.get('span').text())

const button = (wrapper: VueWrapper, text: string) =>
  wrapper.findAll('button').find(item => item.text() === text)!

describe('ProfileBalanceNotifyCard', () => {
  beforeEach(() => {
    vi.useFakeTimers()
    vi.resetAllMocks()
    sendNotifyEmailCode.mockResolvedValue({})
    getProfile.mockResolvedValue({ balance_notify_extra_emails: [] })
  })

  afterEach(() => {
    vi.clearAllTimers()
    vi.useRealTimers()
  })

  it.each(['remove', 'unmount'])('does not start a pending email timer after %s', async (action) => {
    const request = deferred()
    sendNotifyEmailCode.mockReturnValueOnce(request.promise)
    const wrapper = mount(ProfileBalanceNotifyCard, {
      props: { enabled: true, threshold: null, extraEmails: [], systemDefaultThreshold: 5, userEmail: '' }
    })
    await wrapper.get('input[type="email"]').setValue('new@example.com')
    await button(wrapper, 'common.add').trigger('click')
    await button(wrapper, 'profile.balanceNotify.sendCode').trigger('click')
    if (action === 'remove') await button(wrapper, 'profile.balanceNotify.removeEmail').trigger('click')
    else wrapper.unmount()

    request.resolve()
    await flushPromises()
    expect(vi.getTimerCount()).toBe(0)
  })

  it('does not start a saved email timer after unmount', async () => {
    const request = deferred()
    sendNotifyEmailCode.mockReturnValueOnce(request.promise)
    const wrapper = mount(ProfileBalanceNotifyCard, {
      props: {
        enabled: true, threshold: null, systemDefaultThreshold: 5, userEmail: '',
        extraEmails: [{ email: 'saved@example.com', disabled: false, verified: false }]
      }
    })
    await button(wrapper, 'profile.balanceNotify.verify').trigger('click')
    wrapper.unmount()
    request.resolve()
    await flushPromises()
    expect(vi.getTimerCount()).toBe(0)
  })

  it.each([0, 1])('removes only verified emails when request %i finishes first', async (first) => {
    const emails = ['first@example.com', 'second@example.com', 'third@example.com']
    const requests = [deferred(), deferred()]
    verifyNotifyEmail.mockImplementation((email: string) => requests[emails.indexOf(email)]!.promise)
    const wrapper = mount(ProfileBalanceNotifyCard, {
      props: { enabled: true, threshold: null, extraEmails: [], systemDefaultThreshold: 5, userEmail: '' }
    })

    for (const email of emails) {
      await wrapper.get('input[type="email"]').setValue(email)
      await button(wrapper, 'common.add').trigger('click')
    }
    for (const row of pendingRows(wrapper)) {
      await row.findAll('button').find(item => item.text() === 'profile.balanceNotify.sendCode')!.trigger('click')
    }
    await flushPromises()
    for (const row of pendingRows(wrapper)) {
      await row.get('input').setValue('123456')
    }
    for (const row of pendingRows(wrapper).slice(0, 2)) {
      await row.findAll('button').find(item => item.text() === 'profile.balanceNotify.verify')!.trigger('click')
    }
    expect(verifyNotifyEmail.mock.calls).toEqual(emails.slice(0, 2).map(email => [email, '123456']))

    requests[first]!.resolve()
    await flushPromises()
    expect(pendingEmails(wrapper)).toEqual(emails.filter((_, index) => index !== first))

    requests[1 - first]!.resolve()
    await flushPromises()
    expect(pendingEmails(wrapper)).toEqual([emails[2]])
    expect((pendingRows(wrapper)[0]!.get('input').element as HTMLInputElement).value).toBe('123456')
    expect(getProfile).toHaveBeenCalledTimes(2)
  })
})
