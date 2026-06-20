import { describe, expect, it } from 'vitest'
import {
  chartGrayPalette,
  grayBadgeClass,
  grayBorderClass,
  grayButtonClass,
  grayIconClass,
  grayProgressBarClass,
  grayTextClass,
  modelChipClass,
} from '@/utils/grayTheme'

describe('grayTheme', () => {
  it('returns neutral classes for platform-like badges', () => {
    expect(grayBadgeClass()).toBe(
      'border-gray-300 bg-gray-100 text-gray-700 dark:border-dark-600 dark:bg-dark-700 dark:text-gray-200'
    )
    expect(grayBorderClass()).toBe('border-gray-200 dark:border-dark-600')
  })

  it('keeps buttons monochrome', () => {
    expect(grayButtonClass()).toBe(
      'bg-gray-900 text-white hover:bg-gray-800 active:bg-black dark:bg-gray-100 dark:text-dark-950 dark:hover:bg-white'
    )
  })

  it('uses neutral text and icon classes', () => {
    expect(grayTextClass()).toBe('text-gray-900 dark:text-gray-100')
    expect(grayIconClass()).toBe('text-gray-600 dark:text-gray-300')
  })

  it('uses the same model chip class for all presets', () => {
    expect(modelChipClass()).toContain('bg-gray-100')
    expect(modelChipClass()).toContain('dark:bg-dark-700')
  })

  it('uses grayscale chart colors', () => {
    expect(chartGrayPalette).toEqual([
      '#111827',
      '#374151',
      '#4b5563',
      '#6b7280',
      '#9ca3af',
      '#d1d5db',
      '#525252',
      '#737373',
    ])
  })

  it('keeps quota progress neutral until danger state', () => {
    expect(grayProgressBarClass(20)).toBe('bg-gray-700 dark:bg-gray-300')
    expect(grayProgressBarClass(91)).toBe('bg-red-500')
  })
})
