import { describe, expect, it, beforeEach } from 'vitest'
import { createRouter, createMemoryHistory } from 'vue-router'
import { i18n } from '@/i18n'
import { updateRouteSEO, resolveRouteSEO } from '@/utils/seo'
import type { PublicSettings } from '@/types'

const settings: PublicSettings = {
  registration_enabled: false,
  email_verify_enabled: false,
  force_email_on_third_party_signup: false,
  registration_email_suffix_whitelist: [],
  promo_code_enabled: true,
  password_reset_enabled: false,
  invitation_code_enabled: false,
  frontend_url: 'https://example.com',
  turnstile_enabled: false,
  turnstile_site_key: '',
  site_name: 'Sub2API',
  site_logo: '/logo.png',
  site_subtitle: 'One Key, All AI Models',
  api_base_url: '',
  contact_info: '',
  doc_url: '',
  home_content: '',
  hide_ccs_import_button: false,
  payment_enabled: false,
  risk_control_enabled: false,
  table_default_page_size: 20,
  table_page_size_options: [10, 20, 50],
  custom_menu_items: [],
  custom_endpoints: [],
  linuxdo_oauth_enabled: false,
  wechat_oauth_enabled: false,
  oidc_oauth_enabled: false,
  oidc_oauth_provider_name: 'OIDC',
  github_oauth_enabled: false,
  google_oauth_enabled: false,
  backend_mode_enabled: false,
  version: '1.0.0',
  balance_low_notify_enabled: false,
  account_quota_notify_enabled: false,
  balance_low_notify_threshold: 0,
  channel_monitor_enabled: true,
  channel_monitor_default_interval_seconds: 60,
  available_channels_enabled: false,
  affiliate_enabled: false,
  login_agreement_documents: [
    { id: 'terms', title: 'Terms of Service', content_md: '' },
  ],
}

describe('seo utils', () => {
  beforeEach(() => {
    document.head.innerHTML = ''
    document.title = ''
    i18n.global.setLocaleMessage('en', {
      home: { heroDescription: 'One API key for every model.' },
      common: { login: 'Login' },
      errors: {
        pageNotFound: 'Page not found',
        pageNotFoundDescription: "The page you are looking for doesn't exist or has been moved.",
      },
    })
    i18n.global.locale.value = 'en'
  })

  it('resolves home seo as indexable', async () => {
    const router = createRouter({
      history: createMemoryHistory(),
      routes: [{ path: '/home', name: 'Home', component: { template: '<div />' }, meta: { requiresAuth: false, title: 'Home' } }],
    })
    await router.push('/home')
    const seo = resolveRouteSEO(router.currentRoute.value, settings)
    expect(seo.title).toBe('Sub2API - AI API Gateway')
    expect(seo.robots).toBe('index, follow')
    expect(seo.canonicalUrl).toBe('https://example.com/')
    expect(seo.imageUrl).toBe('https://example.com/logo.png')
  })

  it('updates document head tags', async () => {
    const router = createRouter({
      history: createMemoryHistory(),
      routes: [{ path: '/legal/:documentId', name: 'LegalDocument', component: { template: '<div />' }, meta: { requiresAuth: false, title: 'Legal Document' } }],
    })
    await router.push('/legal/terms')

    updateRouteSEO(router.currentRoute.value, settings)

    expect(document.title).toBe('Terms of Service - Sub2API')
    expect(document.head.querySelector('meta[name="description"]')?.getAttribute('content')).toContain('Terms of Service')
    expect(document.head.querySelector('link[rel="canonical"]')?.getAttribute('href')).toBe('https://example.com/legal/terms')
    expect(document.head.querySelector('meta[property="og:title"]')?.getAttribute('content')).toBe('Terms of Service - Sub2API')
    expect(document.head.querySelector('meta[property="og:image"]')?.getAttribute('content')).toBe('https://example.com/og/legal-terms.svg')
  })

  it('marks explicit 404 route as noindex', async () => {
    const router = createRouter({
      history: createMemoryHistory(),
      routes: [{ path: '/:pathMatch(.*)*', name: 'NotFound', component: { template: '<div />' }, meta: { requiresAuth: false, title: '404 Not Found' } }],
    })
    await router.push('/does-not-exist')

    const seo = resolveRouteSEO(router.currentRoute.value, settings)
    expect(seo.robots).toBe('noindex, nofollow')
    expect(seo.title).toBe('Sub2API - Page not found')
    expect(seo.description).toBeTruthy()

    updateRouteSEO(router.currentRoute.value, settings)
    expect(document.head.querySelector('meta[name="robots"]')?.getAttribute('content')).toBe('noindex, nofollow')
  })

  it('marks missing legal document route as noindex', async () => {
    const router = createRouter({
      history: createMemoryHistory(),
      routes: [{ path: '/legal/:documentId', name: 'LegalDocument', component: { template: '<div />' }, meta: { requiresAuth: false, title: 'Legal Document' } }],
    })
    await router.push('/legal/missing')

    const seo = resolveRouteSEO(router.currentRoute.value, settings)
    expect(seo.robots).toBe('noindex, nofollow')
    expect(seo.title).toBe('Sub2API - Page not found')
  })

  it('marks missing custom page route as noindex', async () => {
    const router = createRouter({
      history: createMemoryHistory(),
      routes: [{ path: '/custom/:id', name: 'CustomPage', component: { template: '<div />' }, meta: { requiresAuth: false, title: 'Custom Page' } }],
    })
    await router.push('/custom/missing')

    const seo = resolveRouteSEO(router.currentRoute.value, settings)
    expect(seo.robots).toBe('noindex, nofollow')
    expect(seo.title).toBe('Sub2API - Page not found')
  })

  it('resolves tutorial seo with readable chinese title and description', async () => {
    const router = createRouter({
      history: createMemoryHistory(),
      routes: [{ path: '/docs/tutorial', name: 'TutorialDocument', component: { template: '<div />' }, meta: { requiresAuth: false, title: 'Tutorial Document' } }],
    })
    await router.push('/docs/tutorial')

    const seo = resolveRouteSEO(router.currentRoute.value, {
      ...settings,
      seo_default_title: '',
      seo_default_description: '',
    })

    expect(seo.title).toBe('教程文档 - Sub2API')
    expect(seo.description).toContain('阅读 教程文档')
    expect(seo.canonicalUrl).toBe('https://example.com/docs/tutorial')
    expect(seo.imageUrl).toBe('https://example.com/og/custom-tutorial.svg')
    expect(seo.robots).toBe('index, follow')
  })

  it('omits canonical and og:url when frontend_url is not configured', async () => {
    const router = createRouter({
      history: createMemoryHistory(),
      routes: [{ path: '/home', name: 'Home', component: { template: '<div />' }, meta: { requiresAuth: false, title: 'Home' } }],
    })
    await router.push('/home')

    updateRouteSEO(router.currentRoute.value, {
      ...settings,
      frontend_url: '',
      site_logo: '/logo.png',
      seo_default_og_image: '',
    })

    expect(document.head.querySelector('link[rel="canonical"]')).toBeNull()
    expect(document.head.querySelector('meta[property="og:url"]')).toBeNull()
    expect(document.head.querySelector('meta[property="og:image"]')).toBeNull()
  })

  it('reuses existing SSR json-ld script instead of duplicating it', async () => {
    document.head.innerHTML = '<script type="application/ld+json">{"@type":"Article"}</script>'
    const router = createRouter({
      history: createMemoryHistory(),
      routes: [{ path: '/home', name: 'Home', component: { template: '<div />' }, meta: { requiresAuth: false, title: 'Home' } }],
    })
    await router.push('/home')

    updateRouteSEO(router.currentRoute.value, settings)

    const scripts = document.head.querySelectorAll('script[type="application/ld+json"]')
    expect(scripts).toHaveLength(1)
    expect(scripts[0].id).toBe('route-jsonld')
    expect(scripts[0].textContent).toContain('"@type":"WebSite"')
  })
})
