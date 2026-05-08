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
    expect(seo.canonicalUrl).toBe('https://example.com/home')
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
  })
})
