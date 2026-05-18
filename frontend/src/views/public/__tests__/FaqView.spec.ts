import { describe, it, expect, beforeEach } from 'vitest'
import { mount } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import { createI18n } from 'vue-i18n'
import { createMemoryHistory, createRouter } from 'vue-router'
import FaqView from '../FaqView.vue'

const router = createRouter({
  history: createMemoryHistory(),
  routes: [{ path: '/home', component: { template: '<div/>' } }]
})

describe('FaqView', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
  })

  it('renders at least 10 FAQ items in zh', () => {
    const i18n = createI18n({ legacy: false, locale: 'zh', fallbackLocale: 'zh', messages: { zh: {}, en: {} } })
    const wrapper = mount(FaqView, { global: { plugins: [router, i18n] } })
    const items = wrapper.findAll('dt')
    expect(items.length).toBeGreaterThanOrEqual(10)
    expect(items[0].text()).toBeTruthy()
  })

  it('switches to en items when locale is en', () => {
    const i18n = createI18n({ legacy: false, locale: 'en', fallbackLocale: 'en', messages: { zh: {}, en: {} } })
    const wrapper = mount(FaqView, { global: { plugins: [router, i18n] } })
    expect(wrapper.text()).toContain('Sub2API')
    expect(wrapper.text()).toContain('Frequently Asked Questions')
  })

  it('each item has an id anchor on dt', () => {
    const i18n = createI18n({ legacy: false, locale: 'zh', fallbackLocale: 'zh', messages: { zh: {}, en: {} } })
    const wrapper = mount(FaqView, { global: { plugins: [router, i18n] } })
    const dts = wrapper.findAll('dt')
    for (const dt of dts) {
      expect(dt.attributes('id')).toBeTruthy()
    }
  })
})
