import { beforeEach, describe, expect, it, vi } from 'vitest'
import { createPinia, setActivePinia } from 'pinia'
import type { AdminComplianceStatus } from '@/api/admin/compliance'
import { useAdminComplianceStore } from '../adminCompliance'

const api = vi.hoisted(() => ({ getStatus: vi.fn(), accept: vi.fn() }))
vi.mock('@/api/admin/compliance', () => ({ default: api }))
vi.mock('@/i18n', () => ({ getLocale: () => 'en' }))

function status(required: boolean, version = 'current'): AdminComplianceStatus {
  return {
    required, version, document_path_zh: '', document_path_en: '',
    document_url_zh: '', document_url_en: '', ack_phrase_zh: '测试', ack_phrase_en: 'test'
  }
}

function deferred() {
  let resolve!: (value: AdminComplianceStatus) => void
  let reject!: (error: Error) => void
  const promise = new Promise<AdminComplianceStatus>((yes, no) => { resolve = yes; reject = no })
  return { promise, resolve, reject }
}

beforeEach(() => {
  setActivePinia(createPinia())
  vi.resetAllMocks()
})

describe.each(['fetch', 'accept'] as const)('compliance %s across logout', (action) => {
  const mock = () => action === 'fetch' ? api.getStatus : api.accept
  const run = (store: ReturnType<typeof useAdminComplianceStore>) =>
    action === 'fetch' ? store.fetchStatus() : store.accept('test')
  const busy = (store: ReturnType<typeof useAdminComplianceStore>) =>
    action === 'fetch' ? store.loading : store.submitting

  it('does not repopulate cleared state when an old request succeeds', async () => {
    const pending = deferred()
    mock().mockReturnValueOnce(pending.promise)
    const store = useAdminComplianceStore()
    const request = run(store)
    store.reset()
    const response = status(true, 'old')
    pending.resolve(response)
    expect(await request).toEqual(response)
    expect(store.status).toBeNull()
    expect(store.initialized).toBe(false)
    expect(store.shouldShow).toBe(false)
    expect(busy(store)).toBe(false)
  })

  it.each(['resolve', 'reject'] as const)('keeps a new session pending when the old request %s', async (outcome) => {
    const old = deferred()
    const current = deferred()
    mock().mockReturnValueOnce(old.promise).mockReturnValueOnce(current.promise)
    const store = useAdminComplianceStore()
    const first = run(store).catch(error => error)
    store.reset()
    const second = run(store)
    if (outcome === 'resolve') old.resolve(status(false, 'old'))
    else old.reject(new Error('old failure'))
    await first
    expect(busy(store)).toBe(true)
    expect(store.status).toBeNull()
    current.resolve(status(true))
    await second
    expect(busy(store)).toBe(false)
    expect(store.status).toEqual(status(true))
    expect(store.shouldShow).toBe(true)
  })
})

it('still updates compliance status during a normal fetch and acceptance', async () => {
  api.getStatus.mockResolvedValue(status(true))
  api.accept.mockResolvedValue(status(false))
  const store = useAdminComplianceStore()
  await store.fetchStatus()
  expect(store.initialized).toBe(true)
  expect(store.shouldShow).toBe(true)
  await store.accept('test')
  expect(api.accept).toHaveBeenCalledWith({ phrase: 'test', language: 'en' })
  expect(store.status).toEqual(status(false))
  expect(store.shouldShow).toBe(false)
})
