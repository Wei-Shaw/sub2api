import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

const getMapping = vi.hoisted(() => vi.fn())
vi.mock('@/api/admin/accounts', () => ({ getAntigravityDefaultModelMapping: getMapping }))

beforeEach(() => {
  vi.resetModules()
  getMapping.mockReset()
  vi.spyOn(console, 'warn').mockImplementation(() => {})
})
afterEach(() => vi.restoreAllMocks())

describe('Antigravity default mapping cache', () => {
  it('retries after a failed fetch rather than caching the empty fallback', async () => {
    getMapping.mockRejectedValueOnce(new Error('offline')).mockResolvedValueOnce({ alias: 'upstream-model' })
    const { fetchAntigravityDefaultMappings: fetch } = await import('../useModelWhitelist')
    expect(await fetch()).toEqual([])
    expect(await fetch()).toEqual([{ from: 'alias', to: 'upstream-model' }])
    expect(getMapping).toHaveBeenCalledTimes(2)
  })

  it('caches a successful mapping', async () => {
    getMapping.mockResolvedValue({ alias: 'upstream-model' })
    const { fetchAntigravityDefaultMappings: fetch } = await import('../useModelWhitelist')
    expect(await fetch()).toEqual([{ from: 'alias', to: 'upstream-model' }])
    expect(await fetch()).toEqual([{ from: 'alias', to: 'upstream-model' }])
    expect(getMapping).toHaveBeenCalledTimes(1)
  })

  it('still caches a successful empty mapping', async () => {
    getMapping.mockResolvedValue({})
    const { fetchAntigravityDefaultMappings: fetch } = await import('../useModelWhitelist')
    expect(await fetch()).toEqual([])
    expect(await fetch()).toEqual([])
    expect(getMapping).toHaveBeenCalledTimes(1)
  })

  it('does not erase a successful concurrent result when an older request fails', async () => {
    let reject!: (error: Error) => void
    getMapping.mockImplementationOnce(() => new Promise((_, fail) => { reject = fail }))
      .mockResolvedValueOnce({ alias: 'upstream-model' })
    const { fetchAntigravityDefaultMappings: fetch } = await import('../useModelWhitelist')
    const older = fetch()
    await fetch()
    reject(new Error('offline'))
    expect(await older).toEqual([])
    expect(await fetch()).toEqual([{ from: 'alias', to: 'upstream-model' }])
    expect(getMapping).toHaveBeenCalledTimes(2)
  })
})
