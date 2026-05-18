import { describe, it, expect, beforeEach } from 'vitest'
import { mount } from '@vue/test-utils'
import { defineComponent, h, nextTick } from 'vue'
import { createPinia, setActivePinia } from 'pinia'
import { createI18n } from 'vue-i18n'
import { createMemoryHistory, createRouter, useRoute } from 'vue-router'
import { useSeoHead } from '../useSeoHead'

const i18n = createI18n({
  legacy: false,
  locale: 'zh',
  fallbackLocale: 'zh',
  messages: { zh: {}, en: {} }
})

const router = createRouter({
  history: createMemoryHistory(),
  routes: [
    { path: '/home', component: { template: '<div/>' } },
    { path: '/login', component: { template: '<div/>' } }
  ]
})

const Probe = defineComponent({
  setup() {
    useSeoHead()
    const r = useRoute()
    return () => h('div', r.path)
  }
})

function makeMount() {
  setActivePinia(createPinia())
  return mount(Probe, { global: { plugins: [router, i18n] } })
}

function clearHead() {
  while (document.head.firstChild) {
    document.head.removeChild(document.head.firstChild)
  }
}

describe('useSeoHead', () => {
  beforeEach(() => {
    clearHead()
    document.title = ''
  })

  it('sets title and meta on initial mount for /home', async () => {
    await router.push('/home')
    makeMount()
    await nextTick()
    expect(document.title).toContain('Sub2API')
    expect(document.querySelector('meta[name="description"]')?.getAttribute('content')).toBeTruthy()
    expect(document.querySelector('meta[name="robots"]')?.getAttribute('content')).toBe('index,follow')
  })

  it('switches meta when route changes to /login (noindex)', async () => {
    await router.push('/home')
    makeMount()
    await nextTick()
    await router.push('/login')
    await nextTick()
    expect(document.querySelector('meta[name="robots"]')?.getAttribute('content')).toBe('noindex,follow')
  })
})
