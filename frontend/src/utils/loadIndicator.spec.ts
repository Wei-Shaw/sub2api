import { describe, it, expect } from 'vitest'
import { getLoadBarClass, getLoadBarStyle, getLoadTextClass } from './loadIndicator'

describe('loadIndicator.getLoadBarClass', () => {
  it('returns green for low load (0%)', () => {
    expect(getLoadBarClass(0)).toBe('bg-green-500 dark:bg-green-600')
  })
  it('returns green just below yellow threshold (49.9%)', () => {
    expect(getLoadBarClass(49.9)).toBe('bg-green-500 dark:bg-green-600')
  })
  it('returns yellow at 50%', () => {
    expect(getLoadBarClass(50)).toBe('bg-yellow-500 dark:bg-yellow-600')
  })
  it('returns yellow at 70 boundary minus epsilon', () => {
    expect(getLoadBarClass(69.99)).toBe('bg-yellow-500 dark:bg-yellow-600')
  })
  it('returns orange at 70%', () => {
    expect(getLoadBarClass(70)).toBe('bg-orange-500 dark:bg-orange-600')
  })
  it('returns orange just below red threshold (89.9%)', () => {
    expect(getLoadBarClass(89.9)).toBe('bg-orange-500 dark:bg-orange-600')
  })
  it('returns red at 90%', () => {
    expect(getLoadBarClass(90)).toBe('bg-red-500 dark:bg-red-600')
  })
  it('returns red at 100%', () => {
    expect(getLoadBarClass(100)).toBe('bg-red-500 dark:bg-red-600')
  })
  it('returns red beyond 100%', () => {
    expect(getLoadBarClass(150)).toBe('bg-red-500 dark:bg-red-600')
  })
  it('returns green for NaN (defensive)', () => {
    expect(getLoadBarClass(Number.NaN)).toBe('bg-green-500 dark:bg-green-600')
  })
  it('returns green for negative values', () => {
    expect(getLoadBarClass(-10)).toBe('bg-green-500 dark:bg-green-600')
  })
})

describe('loadIndicator.getLoadBarStyle', () => {
  it('clamps 0', () => {
    expect(getLoadBarStyle(0)).toBe('width: 0%')
  })
  it('mid value (50)', () => {
    expect(getLoadBarStyle(50)).toBe('width: 50%')
  })
  it('full (100)', () => {
    expect(getLoadBarStyle(100)).toBe('width: 100%')
  })
  it('clamps over 100', () => {
    expect(getLoadBarStyle(250)).toBe('width: 100%')
  })
  it('clamps negative to 0', () => {
    expect(getLoadBarStyle(-30)).toBe('width: 0%')
  })
  it('clamps NaN to 0', () => {
    expect(getLoadBarStyle(Number.NaN)).toBe('width: 0%')
  })
})

describe('loadIndicator.getLoadTextClass', () => {
  it('returns green for low', () => {
    expect(getLoadTextClass(0)).toBe('text-green-600 dark:text-green-400')
  })
  it('returns yellow at 50', () => {
    expect(getLoadTextClass(50)).toBe('text-yellow-600 dark:text-yellow-400')
  })
  it('returns orange at 70', () => {
    expect(getLoadTextClass(70)).toBe('text-orange-600 dark:text-orange-400')
  })
  it('returns red at 90', () => {
    expect(getLoadTextClass(90)).toBe('text-red-600 dark:text-red-400')
  })
  it('returns red at 100+', () => {
    expect(getLoadTextClass(100)).toBe('text-red-600 dark:text-red-400')
    expect(getLoadTextClass(150)).toBe('text-red-600 dark:text-red-400')
  })
  it('returns green for NaN', () => {
    expect(getLoadTextClass(Number.NaN)).toBe('text-green-600 dark:text-green-400')
  })
})
