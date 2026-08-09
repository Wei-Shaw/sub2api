import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import { i18n } from '@/i18n'
import { formatRelativeTime, formatRelativeWithDateTime } from '@/utils/format'

describe('formatRelativeTime', () => {
  const originalLocale = i18n.global.locale.value
  const originalEnglishMessages = i18n.global.getLocaleMessage('en')
  const originalChineseMessages = i18n.global.getLocaleMessage('zh')

  beforeEach(() => {
    vi.useFakeTimers()
    vi.setSystemTime(new Date('2026-08-10T00:10:32Z'))
    i18n.global.setLocaleMessage('en', {})
    i18n.global.setLocaleMessage('zh', {})
  })

  afterEach(() => {
    vi.useRealTimers()
    i18n.global.setLocaleMessage('en', originalEnglishMessages)
    i18n.global.setLocaleMessage('zh', originalChineseMessages)
    i18n.global.locale.value = originalLocale
  })

  it('语言消息尚未加载时使用中文回退文案', () => {
    i18n.global.locale.value = 'zh'

    expect(formatRelativeTime('2026-08-10T00:05:32Z')).toBe('5分钟前')
    expect(formatRelativeWithDateTime('2026-08-10T00:05:32Z')).not.toContain('common.time.')
  })

  it('语言消息尚未加载时使用英文回退文案', () => {
    i18n.global.locale.value = 'en'

    expect(formatRelativeTime('2026-08-10T00:05:32Z')).toBe('5m ago')
    expect(formatRelativeTime(null)).toBe('Never')
  })
})
