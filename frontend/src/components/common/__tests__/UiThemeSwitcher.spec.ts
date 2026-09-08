import { beforeEach, describe, expect, it, vi } from 'vitest'
import { mount } from '@vue/test-utils'

import UiThemeSwitcher from '../UiThemeSwitcher.vue'
import { initUiTheme } from '@/composables/useUiTheme'

vi.mock('vue-i18n', async (importOriginal) => {
  const actual = await importOriginal<typeof import('vue-i18n')>()
  return {
    ...actual,
    useI18n: () => ({ t: (key: string) => key }),
  }
})

describe('UiThemeSwitcher', () => {
  beforeEach(() => {
    localStorage.clear()
    initUiTheme()
  })

  it('renders both interface options and marks the current option', () => {
    const wrapper = mount(UiThemeSwitcher)
    const buttons = wrapper.findAll('button')

    expect(buttons.map((button) => button.text())).toEqual([
      'nav.uiThemeOriginal',
      'nav.uiThemeSimple',
    ])
    expect(buttons[0].attributes('aria-pressed')).toBe('true')
    expect(buttons[1].attributes('aria-pressed')).toBe('false')
  })

  it('switches to the simple interface', async () => {
    const wrapper = mount(UiThemeSwitcher)

    await wrapper.findAll('button')[1].trigger('click')

    expect(document.documentElement.dataset.uiTheme).toBe('simple')
    expect(localStorage.getItem('ui-theme')).toBe('simple')
    expect(wrapper.findAll('button')[1].attributes('aria-pressed')).toBe('true')
  })
})
