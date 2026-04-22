import { beforeEach, describe, expect, it, vi REDACTED from 'vitest'

const { get, post REDACTED = vi.hoisted(() => ({
  get: vi.fn(),
  post: vi.fn(),
REDACTED))

vi.mock('@/api/client', () => ({
  apiClient: {
    get,
    post,
  REDACTED,
REDACTED))

import { paymentAPI REDACTED from '@/api/payment'

describe('payment api', () => {
  beforeEach(() => {
    get.mockReset()
    post.mockReset()
    get.mockResolvedValue({ data: {REDACTED REDACTED)
    post.mockResolvedValue({ data: {REDACTED REDACTED)
  REDACTED)

  it('does not expose anonymous public out_trade_no verification', () => {
    expect(Object.prototype.hasOwnProperty.call(paymentAPI, 'verifyOrderPublic')).toBe(false)
  REDACTED)

  it('keeps signed public resume-token resolve endpoint', async () => {
    await paymentAPI.resolveOrderPublicByResumeToken('resume-token-123')

    expect(post).toHaveBeenCalledWith('/payment/public/orders/resolve', {
      resume_token: 'resume-token-123',
    REDACTED)
  REDACTED)
REDACTED)
