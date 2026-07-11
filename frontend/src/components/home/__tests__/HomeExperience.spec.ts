import { readFileSync } from 'node:fs'
import { dirname, resolve } from 'node:path'
import { fileURLToPath } from 'node:url'

import { describe, expect, it } from 'vitest'

const dir = resolve(dirname(fileURLToPath(import.meta.url)), '..')
const navigation = readFileSync(resolve(dir, 'HomeNavigation.vue'), 'utf8')
const hero = readFileSync(resolve(dir, 'HomeHero.vue'), 'utf8')
const apiPreview = readFileSync(resolve(dir, 'HomeApiPreview.vue'), 'utf8')
const capabilities = readFileSync(resolve(dir, 'HomeCapabilities.vue'), 'utf8')

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
    expect(apiPreview).toContain('/v1/chat/completions')
    expect(apiPreview).toContain('grid-template-rows: 235px 165px')
    expect(apiPreview).toContain('142 MS')
    expect(apiPreview).not.toContain('@click')
  })

  it('uses a responsive glass material and disables lift for reduced motion', () => {
    expect(apiPreview).toContain('backdrop-filter: blur(8px) saturate(120%)')
    expect(apiPreview).toContain('.mac-home--dark .api-preview')
    const reducedMotion = apiPreview.slice(apiPreview.indexOf('@media (prefers-reduced-motion: reduce)'))
    expect(reducedMotion).toContain('transform: none;')
  })

  it('exposes only real landing sections and authentication CTA routes', () => {
    expect(capabilities).toContain('id="capabilities"')
    expect(capabilities).toContain('id="models"')
    expect(capabilities).toContain("isAuthenticated ? dashboardPath : '/login'")
    expect(capabilities).not.toContain('to="/keys"')
    expect(capabilities).not.toContain('to="/usage"')
  })
})
