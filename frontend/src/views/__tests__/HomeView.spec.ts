import { readFileSync } from 'node:fs'
import { dirname, resolve } from 'node:path'
import { fileURLToPath } from 'node:url'

import { describe, expect, it } from 'vitest'

const source = readFileSync(
  resolve(dirname(fileURLToPath(import.meta.url)), '../HomeView.vue'),
  'utf8'
)

describe('HomeView macOS landing page assembly', () => {
  it('preserves configured URL and HTML home content overrides', () => {
    expect(source).toContain('v-if="homeContent"')
    expect(source).toContain('v-if="isHomeContentUrl"')
    expect(source).toContain('v-html="homeContent"')
  })

  it('assembles the navigation, hero, capabilities, and footer components', () => {
    for (const component of [
      'HomeNavigation',
      'HomeHero',
      'HomeCapabilities',
      'HomeFooter'
    ]) {
      expect(source).toContain(`<${component}`)
      expect(source).toContain(`@/components/home/${component}.vue`)
    }
  })

  it('uses deployment configuration and never hardcodes localhost', () => {
    expect(source).toContain('cachedPublicSettings?.site_name')
    expect(source).toContain('cachedPublicSettings?.site_logo')
    expect(source).toContain('cachedPublicSettings?.site_subtitle')
    expect(source).toContain('cachedPublicSettings?.doc_url')
    expect(source).toContain('cachedPublicSettings?.api_base_url')
    expect(source).toContain('window.location.origin')
    expect(source).not.toContain('localhost:3000')
  })

  it('supports light/dark mode and reduced motion', () => {
    expect(source).toContain("localStorage.setItem('theme'")
    expect(source).toContain("prefers-color-scheme: dark")
    expect(source).toContain('@media (prefers-reduced-motion: reduce)')
  })
})
