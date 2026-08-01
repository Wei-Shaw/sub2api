import { describe, expect, it } from 'vitest'

import en from '@/i18n/locales/en'
import ko from '@/i18n/locales/ko'
import vi from '@/i18n/locales/vi'
import zhLocale from '@/i18n/locales/zh'
import { formatOpsLogMessage, opsMessageTranslationKeys, prettyOpsJSON, translateOpsText } from '../opsMessageTranslations'

const zh = (hex: string): string =>
  hex
    .split(/\s+/)
    .filter(Boolean)
    .map((part) => String.fromCharCode(Number.parseInt(part, 16)))
    .join('')

const dictionary: Record<string, string> = {
  'admin.ops.messageTranslations.apiKeyGroupDeleted': 'Nhóm của API key đã bị xóa',
  'admin.ops.messageTranslations.apiKeyQuotaExhausted': 'API key đã dùng hết hạn mức',
  'admin.ops.messageTranslations.dailyReport': 'Báo cáo ngày',
  'admin.ops.messageTranslations.dashboardCacheReadFailed': 'Đọc cache dashboard thất bại',
  'admin.ops.messageTranslations.openaiInstructionsRequired': 'OpenAI upstream trả lỗi Instructions are required; đã ghi chi tiết request để điều tra'
}

const t = (key: string): string => dictionary[key] ?? key

describe('opsMessageTranslations', () => {
  it('exports unique translation keys for all configured ops patterns', () => {
    expect(opsMessageTranslationKeys.length).toBeGreaterThan(20)
    expect(new Set(opsMessageTranslationKeys).size).toBe(opsMessageTranslationKeys.length)
  })

  it('defines every dynamic ops message translation key in all supported locales', () => {
    const locales = { en, zh: zhLocale, vi, ko }
    for (const [locale, messages] of Object.entries(locales)) {
      const translations = (messages as any).admin?.ops?.messageTranslations || {}
      for (const key of opsMessageTranslationKeys) {
        expect(translations[key], `${locale}:${key}`).toBeTypeOf('string')
        expect(translations[key], `${locale}:${key}`).not.toEqual('')
      }
    }
  })

  it('translates Chinese business-limit messages shown in ops error logs', () => {
    const raw = `API Key ${zh('6240 5c5e 5206 7ec4 5df2 5220 9664')}`
    expect(translateOpsText(raw, t)).toBe('Nhóm của API key đã bị xóa')
  })

  it('translates locale-neutral English backend messages shown in ops system logs', () => {
    const raw = 'dashboard cache read failed: redis timeout'
    expect(translateOpsText(raw, t)).toBe('Đọc cache dashboard thất bại: redis timeout')
  })

  it('translates English scheduled report names from backend into the active UI locale', () => {
    expect(translateOpsText('Daily report', t)).toBe('Báo cáo ngày')
  })

  it('translates Chinese text inside JSON error bodies before rendering detail', () => {
    const raw = JSON.stringify({ error: { message: `API key ${zh('989d 5ea6 5df2 7528 5b8c')}` } })
    expect(formatOpsLogMessage(raw, t)).toBe('API key đã dùng hết hạn mức')
  })

  it('pretty-prints JSON detail after translating nested string values', () => {
    const raw = JSON.stringify({ report: zh('65e5 62a5') })
    expect(prettyOpsJSON(raw, t)).toContain('Báo cáo ngày')
  })
})
