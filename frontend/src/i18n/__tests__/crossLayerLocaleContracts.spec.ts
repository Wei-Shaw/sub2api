import { describe, expect, it } from 'vitest'

import en from '../locales/en'
import ko from '../locales/ko'
import vi from '../locales/vi'
import zh from '../locales/zh'

type LocaleObject = Record<string, unknown>

function getDottedValue(locale: LocaleObject, dottedKey: string): unknown {
  return dottedKey.split('.').reduce<unknown>((value, segment) => {
    if (!value || typeof value !== 'object' || Array.isArray(value)) {
      return undefined
    }
    return (value as LocaleObject)[segment]
  }, locale)
}

describe('cross-layer locale contracts', () => {
  it('covers Redis ACL setup and weekly payment validity in every runtime locale', () => {
    const requiredKeys = [
      'setup.redis.username',
      'setup.redis.usernamePlaceholder',
      'payment.weeks'
    ]
    const locales = { en, zh, vi, ko }

    const missing = Object.entries(locales).flatMap(([localeName, messages]) =>
      requiredKeys
        .filter(key => {
          const value = getDottedValue(messages, key)
          return typeof value !== 'string' || value.trim() === ''
        })
        .map(key => `${localeName}:${key}`)
    )

    expect(missing).toEqual([])

    const localizedValues = {
      vi: {
        'setup.redis.username': 'Tên người dùng (không bắt buộc)',
        'setup.redis.usernamePlaceholder': 'Để trống để dùng người dùng mặc định',
        'payment.weeks': 'tuần'
      },
      ko: {
        'setup.redis.username': '사용자 이름(선택 사항)',
        'setup.redis.usernamePlaceholder': '기본 사용자는 비워 두세요',
        'payment.weeks': '주'
      }
    }

    for (const [localeName, expectedValues] of Object.entries(localizedValues)) {
      const messages = locales[localeName as keyof typeof locales]
      for (const [key, expected] of Object.entries(expectedValues)) {
        expect(getDottedValue(messages, key), `${localeName}:${key}`).toBe(expected)
      }
    }
  })
})
