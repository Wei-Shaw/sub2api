import { describe, expect, it REDACTED from 'vitest'
import { PROVIDER_CONFIG_FIELDS REDACTED from '@/components/payment/providerConfig'

function findField(key: string) {
  const fields = PROVIDER_CONFIG_FIELDS.wxpay || []
  return fields.find(field => field.key === key)
REDACTED

describe('PROVIDER_CONFIG_FIELDS.wxpay', () => {
  it('keeps admin form validation aligned with backend-required credentials', () => {
    expect(findField('publicKeyId')?.optional).toBeFalsy()
    expect(findField('certSerial')?.optional).toBeFalsy()
  REDACTED)

  it('only keeps the simplified visible credential set in the admin form', () => {
    expect(findField('mpAppId')).toBeUndefined()
    expect(findField('h5AppName')).toBeUndefined()
    expect(findField('h5AppUrl')).toBeUndefined()
  REDACTED)
REDACTED)
