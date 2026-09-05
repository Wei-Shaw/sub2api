import { afterEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount, type VueWrapper } from '@vue/test-utils'
import TurnstileWidget from '../TurnstileWidget.vue'

vi.mock('vue-i18n', () => ({
  useI18n: () => ({ t: (key: string) => key })
}))

const scriptSelector = 'script[src*="challenges.cloudflare.com/turnstile"]'
let wrapper: VueWrapper | undefined

function installSDK() {
  const render = vi.fn<NonNullable<typeof window.turnstile>['render']>(() => 'widget-1')
  window.turnstile = { render, reset: vi.fn(), remove: vi.fn() }
  return render
}

afterEach(() => {
  wrapper?.unmount()
  wrapper = undefined
  delete window.turnstile
  delete window.onTurnstileLoad
  document.querySelectorAll(scriptSelector).forEach((script) => script.remove())
  vi.restoreAllMocks()
})

describe('TurnstileWidget', () => {
  it('shows a loading placeholder until the SDK initializes the widget', async () => {
    wrapper = mount(TurnstileWidget, { props: { siteKey: 'site-key' } })

    expect(wrapper.get('[role="status"]').text()).toBe('auth.captchaLoading')
    expect(document.querySelector(scriptSelector)).not.toBeNull()
    const container = wrapper.get('.turnstile-container').element

    const render = installSDK()
    window.onTurnstileLoad?.()
    await flushPromises()

    expect(render).toHaveBeenCalledWith(container, expect.objectContaining({ sitekey: 'site-key' }))
    expect(wrapper.find('[role="status"]').exists()).toBe(false)
    expect(wrapper.get('.turnstile-container').element).toBe(container)
  })

  it('initializes an already loaded SDK and still forwards verification', async () => {
    const render = installSDK()
    wrapper = mount(TurnstileWidget, { props: { siteKey: 'site-key' } })
    await flushPromises()

    expect(render).toHaveBeenCalledOnce()
    expect(document.querySelector(scriptSelector)).toBeNull()
    expect(wrapper.find('[role="status"]').exists()).toBe(false)

    render.mock.calls[0]![1].callback('verified-token')
    expect(wrapper.emitted('verify')).toEqual([['verified-token']])
  })

  it('ends loading and reports a script load failure', async () => {
    vi.spyOn(console, 'error').mockImplementation(() => {})
    wrapper = mount(TurnstileWidget, { props: { siteKey: 'site-key' } })

    document.querySelector(scriptSelector)!.dispatchEvent(new Event('error'))
    await flushPromises()

    expect(wrapper.find('[role="status"]').exists()).toBe(false)
    expect(wrapper.emitted('error')).toEqual([[]])
  })

  it('does not show a placeholder or load the SDK without a site key', async () => {
    wrapper = mount(TurnstileWidget, { props: { siteKey: '' } })
    await flushPromises()

    expect(wrapper.find('.turnstile-wrapper').exists()).toBe(false)
    expect(wrapper.find('[role="status"]').exists()).toBe(false)
    expect(document.querySelector(scriptSelector)).toBeNull()
  })
})
