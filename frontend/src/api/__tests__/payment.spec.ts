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

  it('keeps legacy public out_trade_no verification for upgrade compatibility', async () => {
    await paymentAPI.verifyOrderPublic('legacy-order-no')

    expect(post).toHaveBeenCalledWith('/payment/public/orders/verify', {
      out_trade_no: 'legacy-order-no',
    REDACTED)
  REDACTED)

  it('keeps signed public resume-token resolve endpoint', async () => {
    await paymentAPI.resolveOrderPublicByResumeToken('resume-token-123')

    expect(post).toHaveBeenCalledWith('/payment/public/orders/resolve', {
      resume_token: 'resume-token-123',
    REDACTED)
  REDACTED)
REDACTED)
