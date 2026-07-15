import { beforeEach, describe, expect, it, vi REDACTED from 'vitest'

const { post REDACTED = vi.hoisted(() => ({
  post: vi.fn()
REDACTED))

vi.mock('@/api/client', () => ({
  apiClient: { post REDACTED
REDACTED))

import { duplicate REDACTED from '@/api/admin/accounts'

describe('admin account duplicate API', () => {
  beforeEach(() => {
    sessionStorage.clear()
    post.mockReset()
    post.mockResolvedValue({ data: { id: 43, name: 'primary (Copy)' REDACTED REDACTED)
    vi.spyOn(globalThis.crypto, 'randomUUID').mockReturnValue('11111111-1111-4111-8111-111111111111')
  REDACTED)

  it('sends a stable idempotency key with the duplicate request', async () => {
    const account = await duplicate(42)

    expect(post).toHaveBeenCalledWith('/admin/accounts/42/duplicate', undefined, {
      headers: {
        'Idempotency-Key': 'account-duplicate-42-11111111-1111-4111-8111-111111111111'
      REDACTED
    REDACTED)
    expect(account).toEqual({ id: 43, name: 'primary (Copy)' REDACTED)
  REDACTED)

  it('reuses the operation key after an ambiguous failed request', async () => {
    post.mockRejectedValueOnce(new Error('network timeout'))
    await expect(duplicate(99)).rejects.toThrow('network timeout')

    post.mockResolvedValueOnce({ data: { id: 100, name: 'retry (Copy)' REDACTED REDACTED)
    await duplicate(99)

    expect(post).toHaveBeenCalledTimes(2)
    const firstHeaders = post.mock.calls[0][2].headers
    const secondHeaders = post.mock.calls[1][2].headers
    expect(secondHeaders).toEqual(firstHeaders)
  REDACTED)

  it('reuses the operation key after a page reload', async () => {
    post.mockRejectedValueOnce(new Error('network timeout'))
    await expect(duplicate(77)).rejects.toThrow('network timeout')
    const firstHeaders = post.mock.calls[0][2].headers

    vi.resetModules()
    post.mockResolvedValueOnce({ data: { id: 78, name: 'reload (Copy)' REDACTED REDACTED)
    const { duplicate: duplicateAfterReload REDACTED = await import('@/api/admin/accounts')
    await duplicateAfterReload(77)

    expect(post).toHaveBeenCalledTimes(2)
    expect(post.mock.calls[1][2].headers).toEqual(firstHeaders)
    expect(sessionStorage.length).toBe(0)
  REDACTED)
REDACTED)
