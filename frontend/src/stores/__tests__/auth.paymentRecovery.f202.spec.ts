import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { createPinia, setActivePinia } from 'pinia'
import { useAuthStore } from '@/stores/auth'
import { PAYMENT_RECOVERY_STORAGE_KEY } from '@/components/payment/paymentFlow'

const f202MockLogin = vi.fn()
const f202MockLogout = vi.fn()
const f202MockGetCurrentUser = vi.fn()

vi.mock('@/api', () => ({
  authAPI: {
    login: (...args: unknown[]) => f202MockLogin(...args),
    logout: (...args: unknown[]) => f202MockLogout(...args),
    getCurrentUser: (...args: unknown[]) => f202MockGetCurrentUser(...args),
  },
  isTotp2FARequired: (response: { requires_2fa?: boolean } | undefined) => response?.requires_2fa === true,
}))

const f202FakeUser = {
  id: 1,
  username: 'buyer',
  email: 'buyer@example.com',
  role: 'user' as const,
  balance: 0,
  concurrency: 5,
  status: 'active' as const,
  allowed_groups: null,
  created_at: '2024-01-01',
  updated_at: '2024-01-01',
}

describe('auth store clears the pending payment snapshot (F2-02)', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
    localStorage.clear()
    vi.useFakeTimers()
    vi.clearAllMocks()
  })

  afterEach(() => {
    vi.useRealTimers()
  })

  it('removes the payment recovery snapshot on logout', async () => {
    f202MockLogin.mockResolvedValue({
      access_token: 'token-a',
      refresh_token: 'refresh-a',
      expires_in: 3600,
      token_type: 'Bearer',
      user: { ...f202FakeUser },
    })
    f202MockLogout.mockResolvedValue(undefined)
    const store = useAuthStore()
    await store.login({ email: 'buyer@example.com', password: 'secret' })
    localStorage.setItem(PAYMENT_RECOVERY_STORAGE_KEY, JSON.stringify({ orderId: 123, userId: 1 }))

    await store.logout()

    expect(localStorage.getItem(PAYMENT_RECOVERY_STORAGE_KEY)).toBeNull()
  })
})
