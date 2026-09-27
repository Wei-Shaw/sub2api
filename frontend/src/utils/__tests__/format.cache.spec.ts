import { afterEach, describe, expect, it, vi } from 'vitest'
import { formatCurrency, formatDate, formatNumber } from '../format'

afterEach(() => {
  vi.restoreAllMocks()
})

describe('utils/format Intl formatter cache', () => {
  it('builds one DateTimeFormat per locale + options and reuses it', () => {
    const ctor = vi.spyOn(Intl, 'DateTimeFormat')
    const options: Intl.DateTimeFormatOptions = { year: 'numeric', month: '2-digit', timeZone: 'Asia/Tokyo' }
    const date = new Date('2026-01-02T03:04:05Z')

    const first = formatDate(date, options, 'en')
    for (let i = 0; i < 100; i++) expect(formatDate(date, { ...options }, 'en')).toBe(first)
    expect(ctor).toHaveBeenCalledTimes(1)

    expect(formatDate(date, options, 'zh')).toBe(new Intl.DateTimeFormat('zh', options).format(date))
    // one cached zh formatter + the reference formatter above
    expect(ctor).toHaveBeenCalledTimes(3)
  })

  it('keeps compact/standard and per-currency number formats apart', () => {
    const ctor = vi.spyOn(Intl, 'NumberFormat')

    expect(formatNumber(1234)).toBe('1,234')
    expect(formatNumber(4321)).toBe('4,321')
    expect(formatNumber(123456)).toBe('123.5K')
    expect(formatNumber(654321)).toBe('654.3K')
    expect(formatCurrency(1.5, 'EUR')).toBe('€1.50')
    expect(formatCurrency(2.5, 'EUR')).toBe('€2.50')
    expect(formatCurrency(1.5, 'JPY')).toBe('¥1.50')

    expect(ctor).toHaveBeenCalledTimes(4)
  })
})
