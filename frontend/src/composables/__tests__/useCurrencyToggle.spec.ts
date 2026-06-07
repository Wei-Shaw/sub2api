import { describe, it, expect, beforeEach } from 'vitest'
import { useCurrencyToggle } from '@/composables/useCurrencyToggle'

const STORAGE_KEY = 'plaza_currency_pref'

describe('useCurrencyToggle', () => {
  beforeEach(() => {
    localStorage.clear()
  })

  it('默认显示币种为 CNY', () => {
    const { display } = useCurrencyToggle(() => 0.14)
    expect(display.value).toBe('CNY')
  })

  it('从 localStorage 读取偏好', () => {
    localStorage.setItem(STORAGE_KEY, 'USD')
    const { display } = useCurrencyToggle(() => 0.14)
    expect(display.value).toBe('USD')
  })

  it('toggle 与 set 切换显示币种并持久化', () => {
    const { display, toggle, set } = useCurrencyToggle(() => 0.14)
    toggle()
    expect(display.value).toBe('USD')
    expect(localStorage.getItem(STORAGE_KEY)).toBe('USD')

    set('CNY')
    expect(display.value).toBe('CNY')
    expect(localStorage.getItem(STORAGE_KEY)).toBe('CNY')
  })

  it('显示币种与 native 一致时不做换算', () => {
    const { convert, set } = useCurrencyToggle(() => 0.14)
    set('CNY')
    expect(convert(100, 'CNY')).toEqual({ value: 100, currency: 'CNY' })
    set('USD')
    expect(convert(2, 'USD')).toEqual({ value: 2, currency: 'USD' })
  })

  it('CNY → USD：amount × multiplier', () => {
    const { convert, set } = useCurrencyToggle(() => 0.1)
    set('USD')
    const { value, currency } = convert(100, 'CNY')
    expect(currency).toBe('USD')
    expect(value).toBeCloseTo(10, 6)
  })

  it('USD → CNY：amount / multiplier', () => {
    const { convert, set } = useCurrencyToggle(() => 0.1)
    set('CNY')
    const { value, currency } = convert(2, 'USD')
    expect(currency).toBe('CNY')
    expect(value).toBeCloseTo(20, 6)
  })

  it('multiplier 为 0/缺失时不换算且回退到原 native 币种', () => {
    const { convert, set } = useCurrencyToggle(() => 0)
    set('USD')
    // CNY native + multiplier=0 → 原值 + CNY
    expect(convert(100, 'CNY')).toEqual({ value: 100, currency: 'CNY' })
  })

  it('multiplier 为负或 NaN 时按缺失处理', () => {
    const { convert: convertNeg } = useCurrencyToggle(() => -1)
    expect(convertNeg(100, 'CNY').currency).toBe('CNY')

    const { convert: convertNaN } = useCurrencyToggle(() => Number.NaN)
    expect(convertNaN(100, 'CNY').currency).toBe('CNY')
  })

  it('format 输出货币符号 + 本地化数字', () => {
    const { format, set } = useCurrencyToggle(() => 0.1)
    set('USD')
    const usd = format(2.5, 'USD', 4)
    expect(usd.startsWith('$')).toBe(true)

    set('CNY')
    const cny = format(100, 'CNY', 2)
    expect(cny.startsWith('¥')).toBe(true)
  })

  it('multiplier 通过 getter 实时取值（典型 meta 异步加载场景）', () => {
    let m: number | null = null
    const { convert, set } = useCurrencyToggle(() => m)

    set('USD')
    // meta 还未加载 → 不换算
    expect(convert(100, 'CNY').currency).toBe('CNY')

    // meta 到达后再换算
    m = 0.2
    expect(convert(100, 'CNY')).toEqual({ value: 20, currency: 'USD' })
  })
})
