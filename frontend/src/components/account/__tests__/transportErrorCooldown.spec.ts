import { describe, expect, it } from 'vitest'
import { isValidTransportErrorCooldownMinutes, readTransportErrorCooldown } from '../transportErrorCooldown'

describe('transport error cooldown settings', () => {
  it.each([undefined, {}, { transport_error_cooldown_enabled: 'false', transport_error_cooldown_minutes: '2' }])(
    'retains safe defaults for missing or malformed stored settings: %s',
    (extra) => expect(readTransportErrorCooldown(extra)).toEqual({ enabled: true, minutes: 10 })
  )

  it('preserves explicit opt-out and custom duration', () => {
    expect(readTransportErrorCooldown({
      transport_error_cooldown_enabled: false,
      transport_error_cooldown_minutes: 2
    })).toEqual({ enabled: false, minutes: 2 })
  })

  it.each([1, 10, 1440])('accepts duration %s', value => {
    expect(isValidTransportErrorCooldownMinutes(value)).toBe(true)
  })

  it.each([0, -1, 1.5, 1441, Infinity, NaN, '', '2', null, undefined])('rejects duration %s', value => {
    expect(isValidTransportErrorCooldownMinutes(value)).toBe(false)
  })
})
