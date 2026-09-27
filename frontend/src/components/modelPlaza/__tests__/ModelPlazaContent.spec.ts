import { describe, expect, it, vi } from 'vitest'
import { mount } from '@vue/test-utils'
import ModelPlazaContent from '../ModelPlazaContent.vue'

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return { ...actual, useI18n: () => ({ t: (key: string) => key }) }
})

vi.mock('@/stores/auth', () => ({
  useAuthStore: () => ({ isAuthenticated: false })
}))

describe('ModelPlazaContent description', () => {
  // Guests on the standalone /model-plaza mount nothing that sets marked's global defaults,
  // so the component must pass breaks: true itself.
  it('renders single newlines as <br> without relying on marked globals', () => {
    const wrapper = mount(ModelPlazaContent, {
      props: { response: { description: 'line one\nline two', groups: [] }, loading: false },
      global: { stubs: { Icon: true, PlazaFilterBar: true, PlazaGroupSection: true } }
    })
    expect(wrapper.find('.plaza-description').html()).toContain('line one<br>line two')
  })
})
