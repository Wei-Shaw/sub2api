import { mount } from '@vue/test-utils'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { ref } from 'vue'

const locale = ref('vi')

vi.mock('vue-i18n', () => ({ useI18n: () => ({ locale }) }))
vi.mock('@/i18n', () => ({
  setLocale: vi.fn(),
  availableLocales: [
    { code: 'en', name: 'English', flag: '🇺🇸' },
    { code: 'vi', name: 'Tiếng Việt', flag: '🇻🇳' },
  ],
}))

import LocaleSwitcher from '../LocaleSwitcher.vue'

let wrapper: ReturnType<typeof mount>
afterEach(() => {
  wrapper?.unmount()
  document.body.innerHTML = ''
})

describe('LocaleSwitcher a11y', () => {
  it('names the native trigger button by the current language and reflects the open state', async () => {
    wrapper = mount(LocaleSwitcher, { attachTo: document.body, global: { stubs: { Icon: true } } })
    const trigger = wrapper.get('button')

    expect(trigger.element.tagName).toBe('BUTTON')
    expect(trigger.attributes('aria-haspopup')).toBe('true')
    expect(trigger.attributes('aria-expanded')).toBe('false')
    expect(trigger.get('.sr-only').text()).toBe('Tiếng Việt')

    await trigger.trigger('click')
    expect(trigger.attributes('aria-expanded')).toBe('true')

    const options = wrapper.findAll('button').slice(1)
    expect(options.map((o) => o.attributes('lang'))).toEqual(['en', 'vi'])
    expect(options.map((o) => o.attributes('aria-current'))).toEqual([undefined, 'true'])
  })

  it('closes on Escape from inside the menu and returns focus to the trigger', async () => {
    wrapper = mount(LocaleSwitcher, { attachTo: document.body, global: { stubs: { Icon: true } } })
    const trigger = wrapper.get('button')
    await trigger.trigger('click')

    const option = wrapper.findAll('button')[1]
    ;(option.element as HTMLButtonElement).focus()
    await option.trigger('keydown', { key: 'Escape' })

    expect(trigger.attributes('aria-expanded')).toBe('false')
    expect(document.activeElement).toBe(trigger.element)
  })
})
