/**
 * useAuthRedirect 组合式函数单元测试
 *
 * Covers the two public branches of `gotoOrLogin`:
 *   1. authenticated user → direct `router.push(target)`
 *   2. anonymous user     → `router.push({ path: '/login', query: { redirect } })`
 *      with `redirect` equal to the resolved `fullPath` of the target (so
 *      nested queries like `?openCreate=1&group_id=42` survive).
 *
 * The composable depends on `useRouter()` and `useAuthStore()`; both are
 * mocked locally so the test stays a pure unit test.
 */
import { describe, it, expect, vi, beforeEach } from 'vitest'
import { setActivePinia, createPinia } from 'pinia'

import { useAuthRedirect } from '../useAuthRedirect'
import { useAuthStore } from '@/stores/auth'

const pushMock = vi.fn().mockResolvedValue(undefined)
const resolveMock = vi.fn()

vi.mock('vue-router', () => ({
  useRouter: () => ({
    push: pushMock,
    resolve: resolveMock,
  }),
}))

describe('useAuthRedirect', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
    pushMock.mockClear()
    resolveMock.mockReset()
  })

  it('authenticated → router.push(target) directly without resolving for /login', async () => {
    const auth = useAuthStore()
    auth.token = 'jwt-token'
    auth.user = { id: 1, username: 'alice', email: 'a@b.c', role: 'user' } as never

    const { gotoOrLogin } = useAuthRedirect()
    const target = { path: '/keys', query: { openCreate: '1', group_id: '42' } }
    await gotoOrLogin(target)

    expect(pushMock).toHaveBeenCalledTimes(1)
    expect(pushMock).toHaveBeenCalledWith(target)
    // The resolve() helper is used only for the anonymous branch.
    expect(resolveMock).not.toHaveBeenCalled()
  })

  it('anonymous → router.push({ path: "/login", query: { redirect } }) with encoded fullPath', async () => {
    // Pinia store starts unauthenticated by default (isAuthenticated checks token + user).
    resolveMock.mockReturnValue({ fullPath: '/keys?openCreate=1&group_id=42' })

    const { gotoOrLogin } = useAuthRedirect()
    const target = { path: '/keys', query: { openCreate: '1', group_id: '42' } }
    await gotoOrLogin(target)

    expect(resolveMock).toHaveBeenCalledWith(target)
    expect(pushMock).toHaveBeenCalledTimes(1)
    expect(pushMock).toHaveBeenCalledWith({
      path: '/login',
      query: { redirect: '/keys?openCreate=1&group_id=42' },
    })
  })
})
