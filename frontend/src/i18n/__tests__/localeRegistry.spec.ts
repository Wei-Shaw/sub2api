import { describe, expect, it } from 'vitest'

import { availableLocales, i18n, loadLocaleMessages } from '../index'

describe('locale registry', () => {
  it('exposes Vietnamese in the language switcher and can load its messages', async () => {
    expect(availableLocales).toContainEqual({ code: 'vi', name: 'Tiếng Việt', flag: '🇻🇳' })

    await loadLocaleMessages('vi')

    expect(i18n.global.availableLocales).toContain('vi')
  })
})
