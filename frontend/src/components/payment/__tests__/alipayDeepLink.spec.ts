import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import {
  ALIPAY_DEEP_LINK_FALLBACK_DELAY_MS,
  ALIPAY_EMBEDDED_BROWSER_FALLBACK_DELAY_MS,
  buildAlipayDeepLink,
  createAlipayDeepLinkLauncher,
} from '../alipayDeepLink'

class FakeEventTarget {
  private readonly listeners = new Map<string, Set<EventListener>>()

  addEventListener(type: string, listener: EventListener) {
    const listeners = this.listeners.get(type) ?? new Set<EventListener>()
    listeners.add(listener)
    this.listeners.set(type, listeners)
  }

  removeEventListener(type: string, listener: EventListener) {
    this.listeners.get(type)?.delete(listener)
  }

  dispatch(type: string) {
    for (const listener of this.listeners.get(type) ?? []) {
      listener(new Event(type))
    }
  }
}

class FakeVisibilityDocument extends FakeEventTarget {
  hidden = false
}

describe('Alipay deep link', () => {
  beforeEach(() => vi.useFakeTimers())
  afterEach(() => vi.useRealTimers())

  it('URL-encodes the dynamic qr_code exactly once', () => {
    const qrCode = 'https://qr.alipay.com/bax123?subject=A B&return=https%3A%2F%2Fexample.com%2Fpaid'
    const deepLink = buildAlipayDeepLink(qrCode)

    expect(deepLink).toBe(
      `alipays://platformapi/startapp?saId=10000007&qrcode=${encodeURIComponent(qrCode)}`,
    )
    expect(decodeURIComponent(deepLink.split('&qrcode=')[1])).toBe(qrCode)
  })

  it('shows fallback after the visible-page timeout', async () => {
    const visibility = new FakeVisibilityDocument()
    const lifecycle = new FakeEventTarget()
    const assignLocation = vi.fn()
    const onStateChange = vi.fn()
    const launcher = createAlipayDeepLinkLauncher({
      qrCode: 'https://qr.alipay.com/dynamic-order-1',
      document: visibility,
      lifecycleTarget: lifecycle,
      userAgent: 'Mozilla/5.0 Mobile Safari',
      assignLocation,
      onStateChange,
    })

    launcher.launch()
    expect(assignLocation).toHaveBeenCalledWith(buildAlipayDeepLink('https://qr.alipay.com/dynamic-order-1'))
    expect(onStateChange).toHaveBeenLastCalledWith('launching')

    await vi.advanceTimersByTimeAsync(ALIPAY_DEEP_LINK_FALLBACK_DELAY_MS - 1)
    expect(onStateChange).not.toHaveBeenCalledWith('fallback')
    await vi.advanceTimersByTimeAsync(1)
    expect(onStateChange).toHaveBeenLastCalledWith('fallback')
  })

  it('keeps fallback hidden when visibilitychange reports the page in background', async () => {
    const visibility = new FakeVisibilityDocument()
    const lifecycle = new FakeEventTarget()
    const onStateChange = vi.fn()
    const launcher = createAlipayDeepLinkLauncher({
      qrCode: 'https://qr.alipay.com/dynamic-order-2',
      document: visibility,
      lifecycleTarget: lifecycle,
      userAgent: 'Mozilla/5.0 iPhone',
      assignLocation: vi.fn(),
      onStateChange,
    })

    launcher.launch()
    visibility.hidden = true
    visibility.dispatch('visibilitychange')
    await vi.advanceTimersByTimeAsync(ALIPAY_DEEP_LINK_FALLBACK_DELAY_MS)

    expect(onStateChange).toHaveBeenLastCalledWith('backgrounded')
    expect(onStateChange).not.toHaveBeenCalledWith('fallback')
  })

  it.each([
    'Mozilla/5.0 MicroMessenger/8.0',
    'Mozilla/5.0 MQQBrowser/13.7 Mobile',
    'Mozilla/5.0 Mobile QQ/9.0',
  ])('uses the fast fallback window in restricted browser %s', async (userAgent) => {
    const visibility = new FakeVisibilityDocument()
    const onStateChange = vi.fn()
    const launcher = createAlipayDeepLinkLauncher({
      qrCode: 'https://qr.alipay.com/dynamic-order-3',
      document: visibility,
      lifecycleTarget: new FakeEventTarget(),
      userAgent,
      assignLocation: vi.fn(),
      onStateChange,
    })

    launcher.launch()
    await vi.advanceTimersByTimeAsync(ALIPAY_EMBEDDED_BROWSER_FALLBACK_DELAY_MS)
    expect(onStateChange).toHaveBeenLastCalledWith('fallback')
  })

  it('treats pagehide as a successful handoff', async () => {
    const visibility = new FakeVisibilityDocument()
    const lifecycle = new FakeEventTarget()
    const onStateChange = vi.fn()
    const launcher = createAlipayDeepLinkLauncher({
      qrCode: 'https://qr.alipay.com/dynamic-order-4',
      document: visibility,
      lifecycleTarget: lifecycle,
      userAgent: 'Mozilla/5.0 Android',
      assignLocation: vi.fn(),
      onStateChange,
    })

    launcher.launch()
    lifecycle.dispatch('pagehide')
    await vi.advanceTimersByTimeAsync(ALIPAY_DEEP_LINK_FALLBACK_DELAY_MS)

    expect(onStateChange).toHaveBeenLastCalledWith('backgrounded')
    expect(onStateChange).not.toHaveBeenCalledWith('fallback')
  })
})
