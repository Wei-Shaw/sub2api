import { describe, expect, it REDACTED from 'vitest'

import { allSelectedGroupsEnableLongContextPricing REDACTED from '../longContextBilling'

const group = (id: number, enabled: boolean) => ({
  id,
  long_context_pricing_enabled: enabled,
REDACTED) as any

describe('allSelectedGroupsEnableLongContextPricing', () => {
  it('hides the account override when every selected group enables tier pricing', () => {
    expect(allSelectedGroupsEnableLongContextPricing(
      [1, 2],
      [group(1, true), group(2, true)]
    )).toBe(true)
  REDACTED)

  it('keeps the account override when any selected group disables tier pricing', () => {
    expect(allSelectedGroupsEnableLongContextPricing(
      [1, 2],
      [group(1, true), group(2, false)]
    )).toBe(false)
  REDACTED)

  it('keeps the account override when selection is empty or group data is incomplete', () => {
    expect(allSelectedGroupsEnableLongContextPricing([], [group(1, true)])).toBe(false)
    expect(allSelectedGroupsEnableLongContextPricing([1, 2], [group(1, true)])).toBe(false)
  REDACTED)
REDACTED)
