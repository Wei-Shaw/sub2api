import { effectScope } from 'vue'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises } from '@vue/test-utils'

vi.mock('@/stores/app', () => ({ useAppStore: () => ({ showError: vi.fn() }) }))
vi.mock('vue-i18n', () => ({ useI18n: () => ({ t: (key: string) => key }) }))
vi.mock('@/api/admin', () => ({
  adminAPI: { cursor: { generateAuthUrl: vi.fn(), poll: vi.fn(), refreshCursorToken: vi.fn() } }
}))

import { adminAPI } from '@/api/admin'
import { useCursorOAuth } from '@/composables/useCursorOAuth'
import type { CursorTokenInfo } from '@/api/admin/cursor'


describe('Cursor OAuth cancellation', () => {
  beforeEach(() => { vi.useFakeTimers(); vi.clearAllMocks() })
  afterEach(() => vi.useRealTimers())

  it('settles a polling delay on reset and sends no later polls', async () => {
    vi.mocked(adminAPI.cursor.poll).mockResolvedValue({ pending: true })
    const oauth = useCursorOAuth()
    oauth.sessionId.value = 'old-session'
    const result = oauth.pollUntilReady()
    await flushPromises()
    oauth.resetState()
    await expect(result).resolves.toBeNull()
    await vi.advanceTimersByTimeAsync(4000)
    expect(adminAPI.cursor.poll).toHaveBeenCalledTimes(1)
    expect(oauth.loading.value).toBe(false)
    expect(oauth.error.value).toBe('')
  })

  it('discards an old in-flight result without clearing the new attempt', async () => {
    const old = Promise.withResolvers<CursorTokenInfo>()
    const fresh = Promise.withResolvers<CursorTokenInfo>()
    vi.mocked(adminAPI.cursor.poll).mockReturnValueOnce(old.promise).mockReturnValueOnce(fresh.promise)
    const oauth = useCursorOAuth()
    oauth.sessionId.value = 'old-session'
    const oldResult = oauth.pollUntilReady()
    oauth.resetState()
    oauth.sessionId.value = 'new-session'
    const newResult = oauth.pollUntilReady()
    old.resolve({ access_token: 'obsolete-identity' })
    await expect(oldResult).resolves.toBeNull()
    expect(oauth.loading.value).toBe(true)
    expect(oauth.sessionId.value).toBe('new-session')
    fresh.resolve({ access_token: 'current-identity' })
    await expect(newResult).resolves.toEqual({ access_token: 'current-identity' })
    expect(oauth.loading.value).toBe(false)
  })

  it('cancels polling when its component scope is disposed', async () => {
    vi.mocked(adminAPI.cursor.poll).mockResolvedValue({ pending: true })
    const scope = effectScope()
    const oauth = scope.run(() => useCursorOAuth())!
    oauth.sessionId.value = 'component-session'
    const result = oauth.pollUntilReady()
    await flushPromises()
    scope.stop()
    await expect(result).resolves.toBeNull()
    await vi.advanceTimersByTimeAsync(4000)
    expect(adminAPI.cursor.poll).toHaveBeenCalledTimes(1)
  })
})
