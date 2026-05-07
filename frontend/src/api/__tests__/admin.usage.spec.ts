import { beforeEach, describe, expect, it, vi } from 'vitest'

const { get } = vi.hoisted(() => ({
  get: vi.fn(),
}))

vi.mock('@/api/client', () => ({
  apiClient: {
    get,
  },
}))

import { searchUsers, type SimpleUser } from '@/api/admin/usage'

type Assert<T extends true> = T
type IsExact<T, U> = (
  (<G>() => G extends T ? 1 : 2) extends (<G>() => G extends U ? 1 : 2)
    ? ((<G>() => G extends U ? 1 : 2) extends (<G>() => G extends T ? 1 : 2) ? true : false)
    : false
)

type ExpectedSimpleUser = {
  id: number
  email: string
  primary_redeem_code?: string | null
  primary_redeem_type?: string | null
  has_device_binding?: boolean
}

const simpleUserContractExact: Assert<IsExact<SimpleUser, ExpectedSimpleUser>> = true

describe('admin usage users api', () => {
  beforeEach(() => {
    get.mockReset()
  })

  it('searches users through the shared admin usage endpoint and preserves DLG/device hints', async () => {
    const users: SimpleUser[] = [
      {
        id: 42,
        email: 'device-user@example.com',
        primary_redeem_code: 'DLG-FN7Y-NJQJ-XNV6',
        primary_redeem_type: 'device_login',
        has_device_binding: true,
      },
    ]
    get.mockResolvedValue({ data: users })

    const result = await searchUsers('DLG-FN7Y')

    expect(get).toHaveBeenCalledWith('/admin/usage/search-users', {
      params: { q: 'DLG-FN7Y' },
    })
    expect(result).toEqual(users)
  })

  it('keeps SimpleUser aligned with the backend selector response contract', () => {
    expect(simpleUserContractExact).toBe(true)
  })
})
