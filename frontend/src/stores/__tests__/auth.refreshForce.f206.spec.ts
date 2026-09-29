import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { createPinia, setActivePinia } from 'pinia'
import { useAuthStore } from '@/stores/auth'

const f206MockLogin = vi.fn()
const f206MockGetCurrentUser = vi.fn()

vi.mock('@/api', () => ({
  authAPI: {
    login: (...args: unknown[]) => f206MockLogin(...args),
    logout: vi.fn(),
    getCurrentUser: (...args: unknown[]) => f206MockGetCurrentUser(...args),
  },
  isTotp2FARequired: (response: { requires_2fa?: boolean } | undefined) => response?.requires_2fa === true,
}))

const f206FakeUser = {
  id: 1,
  username: 'redeemer',
  email: 'redeemer@example.com',
  role: 'user' as const,
  balance: 100,
  concurrency: 5,
  status: 'active' as const,
  allowed_groups: null,
  created_at: '2024-01-01',
  updated_at: '2024-01-01',
}

async function f206LoggedInStore() {
  f206MockLogin.mockResolvedValue({
    access_token: 'token-a',
    refresh_token: 'refresh-a',
    expires_in: 3600,
    token_type: 'Bearer',
    user: { ...f206FakeUser },
  })
  const store = useAuthStore()
  await store.login({ email: 'redeemer@example.com', password: 'secret' })
  return store
}

describe('refreshUser force option (F2-06)', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
    localStorage.clear()
    vi.useFakeTimers()
    vi.clearAllMocks()
    f206MockGetCurrentUser.mockReset()
  })

  afterEach(() => {
    vi.useRealTimers()
  })

  it('does not hand a post-mutation caller the /auth/me request that was already in flight', async () => {
    const store = await f206LoggedInStore()
    let resolveStale!: (value: { data: typeof f206FakeUser }) => void
    f206MockGetCurrentUser
      .mockReturnValueOnce(new Promise((resolve) => { resolveStale = resolve }))
      .mockResolvedValueOnce({ data: { ...f206FakeUser, balance: 110 } })

    // 60 s 轮询先发出的请求（读到的是变更前的余额）
    const stale = store.refreshUser()
    // 兑换成功后的刷新
    const fresh = store.refreshUser({ force: true })
    // 普通调用跟随最新的请求
    const joined = store.refreshUser()
    // 新请求排在旧请求之后，旧响应不会晚到覆盖新数据
    expect(f206MockGetCurrentUser).toHaveBeenCalledTimes(1)

    resolveStale({ data: { ...f206FakeUser, balance: 100 } })

    expect((await stale).balance).toBe(100)
    expect((await fresh).balance).toBe(110)
    expect((await joined).balance).toBe(110)
    expect(store.user?.balance).toBe(110)
    expect(f206MockGetCurrentUser).toHaveBeenCalledTimes(2)

    f206MockGetCurrentUser.mockResolvedValueOnce({ data: { ...f206FakeUser, balance: 120 } })
    expect((await store.refreshUser()).balance).toBe(120)
    expect(f206MockGetCurrentUser).toHaveBeenCalledTimes(3)
  })

  it('still sends a fresh request when the in-flight one fails', async () => {
    const store = await f206LoggedInStore()
    f206MockGetCurrentUser
      .mockRejectedValueOnce({ status: 503, message: 'unavailable' })
      .mockResolvedValueOnce({ data: { ...f206FakeUser, balance: 110 } })

    const stale = store.refreshUser()
    const fresh = store.refreshUser({ force: true })

    await expect(stale).rejects.toMatchObject({ status: 503 })
    expect((await fresh).balance).toBe(110)
    expect(f206MockGetCurrentUser).toHaveBeenCalledTimes(2)
  })

  it('does not send the queued request once the session was cleared', async () => {
    const store = await f206LoggedInStore()
    f206MockGetCurrentUser.mockRejectedValueOnce({ status: 401, message: 'expired' })

    const stale = store.refreshUser()
    const fresh = store.refreshUser({ force: true })

    await expect(stale).rejects.toMatchObject({ status: 401 })
    await expect(fresh).rejects.toThrow('Not authenticated')
    expect(f206MockGetCurrentUser).toHaveBeenCalledTimes(1)
  })

  it('keeps sharing one request between plain concurrent callers', async () => {
    const store = await f206LoggedInStore()
    f206MockGetCurrentUser.mockResolvedValue({ data: f206FakeUser })

    await Promise.all([store.refreshUser(), store.refreshUser()])

    expect(f206MockGetCurrentUser).toHaveBeenCalledTimes(1)
  })
})
