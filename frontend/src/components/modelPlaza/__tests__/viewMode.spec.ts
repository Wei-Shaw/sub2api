import { describe, expect, it } from 'vitest'
import { resolveModelPlazaViewMode } from '../viewMode'

describe('resolveModelPlazaViewMode', () => {
  it('defaults to card view without a saved preference', () => {
    expect(resolveModelPlazaViewMode(null)).toBe('card')
    expect(resolveModelPlazaViewMode('')).toBe('card')
    expect(resolveModelPlazaViewMode('unexpected')).toBe('card')
  })

  it('keeps an explicit list preference', () => {
    expect(resolveModelPlazaViewMode('list')).toBe('list')
  })

  it('keeps an explicit card preference', () => {
    expect(resolveModelPlazaViewMode('card')).toBe('card')
  })
})
