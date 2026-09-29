import { describe, expect, it } from 'vitest'
import { readPaymentRecoverySnapshot } from '@/components/payment/paymentFlow'

function f202RawSnapshot(overrides: Record<string, unknown> = {}): string {
  return JSON.stringify({
    orderId: 123,
    amount: 10,
    qrCode: 'qr-123',
    expiresAt: '2099-01-01T00:10:00.000Z',
    paymentType: 'sepay_bank_transfer',
    payUrl: '',
    outTradeNo: 'sub2_123',
    currency: 'VND',
    paymentEnv: '',
    payAmount: 10,
    orderType: 'balance',
    paymentMode: 'qrcode',
    resumeToken: '',
    createdAt: 1,
    ...overrides,
  })
}

describe('readPaymentRecoverySnapshot user binding (F2-02)', () => {
  it('rejects a snapshot that belongs to another user', () => {
    expect(readPaymentRecoverySnapshot(f202RawSnapshot({ userId: 1 }), { userId: 2 })).toBeNull()
  })

  it('rejects an unowned snapshot once the current user is known', () => {
    expect(readPaymentRecoverySnapshot(f202RawSnapshot(), { userId: 2 })).toBeNull()
  })

  it('restores the snapshot for its owner and keeps the owner id', () => {
    const restored = readPaymentRecoverySnapshot(f202RawSnapshot({ userId: 1 }), { userId: 1 })

    expect(restored?.orderId).toBe(123)
    expect(restored?.userId).toBe(1)
  })
})
