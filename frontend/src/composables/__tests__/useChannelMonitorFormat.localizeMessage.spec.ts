import { describe, expect, it, vi } from 'vitest'
import zhDashboard from '@/i18n/locales/zh/dashboard'
import enDashboard from '@/i18n/locales/en/dashboard'

/**
 * localizeMonitorMessage 的解析契约。
 *
 * 后端把这些诊断文案按固定英文格式拼好并落进 channel_monitor_histories.message，
 * 展示层按格式解析后再走 i18n（详见 composable 内注释）。这里用真实 locale 数据驱动，
 * 所以断言同时覆盖两件事：正则能认出后端格式，且 i18n key 真的存在。
 * 后端改文案格式 / locale 少键，测试都会先红，而不是界面静默退回英文。
 */
const locales: Record<string, Record<string, unknown>> = {
  zh: zhDashboard as unknown as Record<string, unknown>,
  en: enDashboard as unknown as Record<string, unknown>,
}

let activeLocale = 'zh'

function lookup(key: string): string | undefined {
  let node: unknown = locales[activeLocale]
  for (const segment of key.split('.')) {
    if (typeof node !== 'object' || node === null) return undefined
    node = (node as Record<string, unknown>)[segment]
  }
  return typeof node === 'string' ? node : undefined
}

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return {
    ...actual,
    // 用真实 locale 表解析 key，并做最小化的 {param} 插值。
    useI18n: () => ({
      t: (key: string, params?: Record<string, unknown>) => {
        const template = lookup(key)
        if (template === undefined) return key
        if (!params) return template
        return template.replace(/\{(\w+)\}/g, (whole, name: string) =>
          params[name] === undefined ? whole : String(params[name]),
        )
      },
      te: (key: string) => lookup(key) !== undefined,
    }),
  }
})

const { useChannelMonitorFormat } = await import('@/composables/useChannelMonitorFormat')

function localize(locale: 'zh' | 'en', message: string): string {
  activeLocale = locale
  return useChannelMonitorFormat().localizeMonitorMessage(message)
}

describe('localizeMonitorMessage', () => {
  it('localizes group exhaustion counts', () => {
    expect(localize('zh', 'no quota left: 2376/2383 accounts exhausted')).toBe(
      '额度已耗尽：2376/2383 个账号无额度',
    )
    expect(localize('en', 'no quota left: 2376/2383 accounts exhausted')).toBe(
      'No quota left: 2376/2383 accounts exhausted',
    )
  })

  it('localizes all-accounts-unavailable', () => {
    expect(localize('zh', 'quota unavailable for all 5 accounts')).toBe('全部 5 个账号均无法获取额度')
  })

  it('localizes quota-high and translates both tier name segments', () => {
    // credits/total 两段分别走 labels / windows 的既有翻译
    expect(localize('zh', 'quota high: credits/total at 99.9%')).toBe(
      '额度紧张：额度/总量 已用 99.9%',
    )
    // 单段窗口
    expect(localize('zh', 'quota high: 5h at 92.3%')).toBe('额度紧张：5 小时 已用 92.3%')
  })

  it('localizes balance-low with and without an amount', () => {
    expect(localize('zh', 'balance low: 15.5 CNY')).toBe('余额不足：15.5 CNY')
    expect(localize('zh', 'balance low (USD)')).toBe('余额不足（USD）')
  })

  it('localizes config errors', () => {
    expect(localize('zh', 'linked account not found')).toBe('关联账号不存在')
    expect(localize('zh', 'linked group not found')).toBe('关联分组不存在')
    expect(localize('zh', 'linked group has no accounts')).toBe('关联分组内没有账号')
  })

  it('passes through unknown messages and blanks', () => {
    // 前向兼容：后端新增文案不应被吞掉
    expect(localize('zh', 'some brand new upstream error')).toBe('some brand new upstream error')
    expect(localize('zh', '   ')).toBe('')
    expect(localize('zh', '')).toBe('')
  })
})
