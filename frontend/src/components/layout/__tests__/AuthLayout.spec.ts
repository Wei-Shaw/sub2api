import { describe, expect, it, vi } from 'vitest'
import { mount } from '@vue/test-utils'
import AuthLayout from '../AuthLayout.vue'

const appStore = {
  siteName: 'Acme API',
  siteLogo: '',
  cachedPublicSettings: null,
  publicSettingsLoaded: false,
  fetchPublicSettings: vi.fn()
}

vi.mock('vue-i18n', async (importOriginal) => ({
  ...(await importOriginal<typeof import('vue-i18n')>()),
  useI18n: () => ({ t: (key: string) => key })
}))

vi.mock('@/stores', () => ({ useAppStore: () => appStore }))

function mountLayout() {
  return mount(AuthLayout, { global: { stubs: { LocaleSwitcher: { template: '<div data-test="locale" />' } } } })
}

describe('AuthLayout', () => {
  it('offers the language switcher even before public settings load', () => {
    appStore.publicSettingsLoaded = false
    expect(mountLayout().find('[data-test="locale"]').exists()).toBe(true)
  })

  it('treats the logo as decorative next to the site name heading', () => {
    appStore.publicSettingsLoaded = true
    const wrapper = mountLayout()
    expect(wrapper.get('img').attributes('alt')).toBe('')
    expect(wrapper.get('h1').text()).toBe('Acme API')
  })
})
