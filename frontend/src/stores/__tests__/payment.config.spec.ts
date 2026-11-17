import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { createPinia, setActivePinia } from 'pinia'
import { usePaymentStore } from '../payment'
import type { PaymentConfig } from '@/types/payment'

const getConfig = vi.hoisted(() => vi.fn())
vi.mock('@/api/payment', () => ({ paymentAPI: { getConfig } }))

const config = { stripe_publishable_key: 'pk_test_example', payment_enabled: true } as PaymentConfig
function deferred() {
  let resolve!: (value: { data: PaymentConfig }) => void
  let reject!: (error: Error) => void
  const promise = new Promise<{ data: PaymentConfig }>((yes, no) => { resolve = yes; reject = no })
  return { promise, resolve, reject }
}

beforeEach(() => {
  setActivePinia(createPinia())
  getConfig.mockReset()
  vi.spyOn(console, 'error').mockImplementation(() => {})
})
afterEach(() => vi.restoreAllMocks())

describe('payment configuration requests', () => {
  it('waits for the same response for concurrent initial callers', async () => {
    const pending = deferred()
    getConfig.mockReturnValue(pending.promise)
    const store = usePaymentStore()
    const first = store.fetchConfig()
    let finished = false
    const second = store.fetchConfig().then(value => { finished = true; return value })
    await Promise.resolve()
    await Promise.resolve()
    expect(finished).toBe(false)
    expect(store.configLoading).toBe(true)
    expect(getConfig).toHaveBeenCalledTimes(1)
    pending.resolve({ data: config })
    expect(await Promise.all([first, second])).toEqual([config, config])
    expect(store.configLoading).toBe(false)
  })

  it('shares a forced refresh instead of returning stale cached configuration', async () => {
    getConfig.mockResolvedValueOnce({ data: config })
    const store = usePaymentStore()
    await store.fetchConfig()
    const pending = deferred()
    getConfig.mockReturnValueOnce(pending.promise)
    const first = store.fetchConfig(true)
    const second = store.fetchConfig(true)
    const updated = { ...config, stripe_publishable_key: 'pk_test_updated' }
    pending.resolve({ data: updated })
    expect(await Promise.all([first, second])).toEqual([updated, updated])
    expect(getConfig).toHaveBeenCalledTimes(2)
  })

  it('releases the shared request after failure so a later call can retry', async () => {
    const pending = deferred()
    getConfig.mockReturnValueOnce(pending.promise)
    const store = usePaymentStore()
    const first = store.fetchConfig()
    const second = store.fetchConfig()
    pending.reject(new Error('offline'))
    expect(await Promise.all([first, second])).toEqual([null, null])
    expect(store.configLoading).toBe(false)
    expect(store.configLoaded).toBe(false)
    getConfig.mockResolvedValueOnce({ data: config })
    expect(await store.fetchConfig()).toEqual(config)
    expect(getConfig).toHaveBeenCalledTimes(2)
  })

  it('returns cached configuration without another request', async () => {
    getConfig.mockResolvedValue({ data: config })
    const store = usePaymentStore()
    await store.fetchConfig()
    expect(await store.fetchConfig()).toEqual(config)
    expect(getConfig).toHaveBeenCalledTimes(1)
  })
})
