import { mount } from '@vue/test-utils'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { ref } from 'vue'

import LocaleSwitcher from '../LocaleSwitcher.vue'

vi.mock('vue-i18n', () => ({
  useI18n: () => ({
    locale: ref('en'),
  }),
}))

vi.mock('@/i18n', () => ({
  availableLocales: [
    { code: 'en', name: 'English', flag: 'US' },
    { code: 'zh', name: '中文', flag: 'CN' },
  ],
  setLocale: vi.fn().mockResolvedValue(undefined),
}))

afterEach(() => {
  document.body.innerHTML = ''
})

describe('LocaleSwitcher', () => {
  it('点击语言切换按钮会展开菜单项', async () => {
    const wrapper = mount(LocaleSwitcher, {
      attachTo: document.body,
      global: {
        stubs: {
          Icon: true,
        },
      },
    })

    await wrapper.get('button[aria-haspopup="menu"]').trigger('click')

    expect(document.body.textContent).toContain('English')
    expect(document.body.textContent).toContain('中文')

    wrapper.unmount()
  })

  it('鼠标 hover 语言切换按钮会展开菜单项', async () => {
    const wrapper = mount(LocaleSwitcher, {
      attachTo: document.body,
      global: {
        stubs: {
          Icon: true,
        },
      },
    })

    await wrapper.get('[data-test="locale-switcher"]').trigger('mouseenter')

    expect(document.body.textContent).toContain('English')
    expect(document.body.textContent).toContain('中文')

    wrapper.unmount()
  })
})
