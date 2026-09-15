import { readFileSync } from 'node:fs'
import { dirname, resolve } from 'node:path'
import { fileURLToPath } from 'node:url'

import { describe, expect, it } from 'vitest'

const dir = resolve(dirname(fileURLToPath(import.meta.url)), '..')
const navigation = readFileSync(resolve(dir, 'HomeNavigation.vue'), 'utf8')
const hero = readFileSync(resolve(dir, 'HomeHero.vue'), 'utf8')
const apiPreview = readFileSync(resolve(dir, 'HomeApiPreview.vue'), 'utf8')
const capabilities = readFileSync(resolve(dir, 'HomeCapabilities.vue'), 'utf8')
const providerLogo = readFileSync(resolve(dir, 'ProviderLogo.vue'), 'utf8')
const footer = readFileSync(resolve(dir, 'HomeFooter.vue'), 'utf8')

describe('macOS home navigation', () => {
  it('uses one semantic brand link to the deployment-relative home route', () => {
    const brand = navigation.match(
      /<router-link[\s\S]*?data-testid="home-brand-link"[\s\S]*?<\/router-link>/
    )?.[0]

    expect(brand).toBeTruthy()
    expect(brand).toContain('to="/home"')
    expect(brand).toContain('home-brand-logo')
    expect(brand).toContain('home-brand-title')
    expect(brand).not.toContain('localhost')
  })

  it('switches to the floating glass state after 20px using passive rAF scrolling', () => {
    expect(navigation).toContain('window.scrollY > 20')
    expect(navigation).toContain("window.requestAnimationFrame(updateScrollState)")
    expect(navigation).toContain("window.addEventListener('scroll', handleScroll, { passive: true })")
    expect(navigation).toContain('backdrop-filter: blur(36px) saturate(150%)')
  })

  it('supports an accessible mobile menu and reduced motion', () => {
    expect(navigation).toContain(':aria-expanded="menuOpen"')
    expect(navigation).toContain("event.key !== 'Escape'")
    expect(navigation).toContain('@media (prefers-reduced-motion: reduce)')
  })
})

describe('macOS home product experience', () => {
  it('integrates the reference-style API preview as the only hero showcase', () => {
    expect(hero).toContain("import HomeApiPreview from '@/components/home/HomeApiPreview.vue'")
    expect(hero).toContain('<HomeApiPreview :api-base-url="apiBaseUrl" />')
    expect(hero).not.toContain('<div class="product-window">')
  })

  it('uses a configured endpoint and a vertical request/response workbench', () => {
    expect(apiPreview).toContain("props.apiBaseUrl.replace(/\\/+$/, '')")
    for (const endpoint of [
      '/v1/chat/completions',
      '/v1/responses',
      '/v1/messages',
      '/v1beta/models/gemini-2.5-flash:generateContent'
    ]) {
      expect(apiPreview).toContain(endpoint)
    }
    expect(apiPreview).toContain('grid-template-rows: 235px 165px')
    expect(apiPreview).toContain('role="tablist"')
    expect(apiPreview).toContain('role="tab"')
    expect(apiPreview).toContain('@click="activeId = example.id"')
    expect(apiPreview).toContain('handleTabKeydown')
  })

  it('uses a responsive glass material and disables lift for reduced motion', () => {
    expect(apiPreview).toContain('backdrop-filter: blur(20px) saturate(140%)')
    expect(apiPreview).toContain('background: var(--home-glass-strong)')
    expect(apiPreview).toContain('.mac-home--dark .api-preview')
    expect(apiPreview).toContain('background: rgb(5 10 18 / 90%)')
    const reducedMotion = apiPreview.slice(apiPreview.indexOf('@media (prefers-reduced-motion: reduce)'))
    expect(reducedMotion).toContain('transform: none;')
  })

  it('renders the selected API protocol as a rounded animated glass control', () => {
    expect(apiPreview).toContain('border-radius: 9px;')
    expect(apiPreview).toContain('.api-preview__tab--active {')
    expect(apiPreview).toContain('background: var(--home-glass-hover);')
    expect(apiPreview).toContain('inset 0 0 0 1px var(--home-glass-border-strong)')
    expect(apiPreview).toContain('.api-preview__tab--active:active { transform: scale(.97); }')
  })

  it('uses vector provider marks and removes the GitHub footer link', () => {
    expect(capabilities).toContain('<ProviderLogo :provider="provider.id" />')
    expect(providerLogo).toContain("'claude' | 'openai' | 'gemini' | 'antigravity'")
    expect(capabilities).not.toContain('provider-item__mark')
    expect(footer).not.toContain('github.com')
    expect(footer).not.toContain('>GitHub<')
    expect(footer).toContain('v-if="docUrl"')
  })

  it('exposes only real landing sections and authentication CTA routes', () => {
    expect(capabilities).toContain('id="capabilities"')
    expect(capabilities).toContain('id="models"')
    expect(capabilities).toContain("isAuthenticated ? dashboardPath : '/login'")
    expect(capabilities).not.toContain('to="/keys"')
    expect(capabilities).not.toContain('to="/usage"')
  })
})
