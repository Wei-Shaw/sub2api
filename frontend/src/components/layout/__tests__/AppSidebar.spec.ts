import { readFileSync } from 'node:fs'
import { dirname, resolve } from 'node:path'
import { fileURLToPath } from 'node:url'

import { describe, expect, it } from 'vitest'

const componentPath = resolve(dirname(fileURLToPath(import.meta.url)), '../AppSidebar.vue')
const componentSource = readFileSync(componentPath, 'utf8')
const stylePath = resolve(dirname(fileURLToPath(import.meta.url)), '../../../style.css')
const styleSource = readFileSync(stylePath, 'utf8')
const zhLocalePath = resolve(dirname(fileURLToPath(import.meta.url)), '../../../i18n/locales/zh.ts')
const enLocalePath = resolve(dirname(fileURLToPath(import.meta.url)), '../../../i18n/locales/en.ts')
const zhLocaleSource = readFileSync(zhLocalePath, 'utf8')
const enLocaleSource = readFileSync(enLocalePath, 'utf8')

describe('AppSidebar custom SVG styles', () => {
  it('does not override uploaded SVG fill or stroke colors', () => {
    expect(componentSource).toContain('.sidebar-svg-icon {')
    expect(componentSource).toContain('color: currentColor;')
    expect(componentSource).toContain('display: block;')
    expect(componentSource).not.toContain('stroke: currentColor;')
    expect(componentSource).not.toContain('fill: none;')
  })
})

describe('AppSidebar header styles', () => {
  it('does not clip the version badge dropdown', () => {
    const sidebarHeaderBlockMatch = styleSource.match(/\.sidebar-header\s*\{[\s\S]*?\n {2}\}/)
    const sidebarBrandBlockMatch = componentSource.match(/\.sidebar-brand\s*\{[\s\S]*?\n\}/)

    expect(sidebarHeaderBlockMatch).not.toBeNull()
    expect(sidebarBrandBlockMatch).not.toBeNull()
    expect(sidebarHeaderBlockMatch?.[0]).not.toContain('@apply overflow-hidden;')
    expect(sidebarBrandBlockMatch?.[0]).not.toContain('overflow: hidden;')
  })
})

describe('AppSidebar brand link', () => {
  it('renders two router-links to "/" (logo + title) so the click area excludes the version badge', () => {
    // Logo link: visible in both expanded and collapsed states.
    expect(componentSource).toMatch(
      /<router-link\s+to="\/"\s+class="sidebar-brand-link sidebar-brand-link-logo"/
    )
    // Title link: visible only in expanded state, marked aria-hidden + tabindex=-1
    // so screen readers announce a single "go home" link (the logo link).
    expect(componentSource).toMatch(
      /<router-link\s+to="\/"\s+class="sidebar-brand-link sidebar-brand-link-text"[\s\S]*?tabindex="-1"[\s\S]*?aria-hidden="true"/
    )
    // Sidebar-header wrapper is back to a plain <div>, NOT a router-link
    // (so the entire header padding area is no longer a click target).
    expect(componentSource).toMatch(
      /<div class="sidebar-header"\s+:class="\{ 'sidebar-header-collapsed': sidebarCollapsed \}">/
    )
    // Version badge MUST NOT be wrapped by any router-link — it must remain
    // a plain sibling so its dropdown/update button keeps working.
    const headerBlockMatch = componentSource.match(
      /<div class="sidebar-header"[\s\S]*?<\/div>\s*\n\s*<!-- Navigation -->/
    )
    expect(headerBlockMatch).not.toBeNull()
    const headerBlock = headerBlockMatch?.[0] ?? ''
    // Version badge appears in the header.
    expect(headerBlock).toContain('<VersionBadge :version="siteVersion" />')
    // No router-link contains a VersionBadge as descendant.
    expect(headerBlock).not.toMatch(/<router-link[\s\S]*?<VersionBadge[\s\S]*?<\/router-link>/)
  })

  it('exposes a localized accessible name via aria-label and title on the logo link', () => {
    expect(componentSource).toContain(":aria-label=\"t('nav.goHome')\"")
    expect(componentSource).toContain(":title=\"t('nav.goHome')\"")
  })

  it('binds a click handler on both brand links that closes the mobile drawer', () => {
    // Both router-links wire the same handler.
    const clickMatches = componentSource.match(/@click="handleBrandClick"/g)
    expect(clickMatches).not.toBeNull()
    expect(clickMatches?.length).toBeGreaterThanOrEqual(2)
    // The handler implementation calls setMobileOpen(false) when mobileOpen.
    const handlerMatch = componentSource.match(/function handleBrandClick\(\)\s*\{[\s\S]*?\n\}/)
    expect(handlerMatch).not.toBeNull()
    expect(handlerMatch?.[0]).toContain('appStore.setMobileOpen(false)')
  })

  it('declares a hover/focus-visible affordance on the brand link', () => {
    expect(componentSource).toContain('.sidebar-brand-link:hover')
    expect(componentSource).toContain('.sidebar-brand-link:focus-visible')
  })

  it('localizes nav.goHome in zh and en', () => {
    expect(zhLocaleSource).toMatch(/goHome:\s*'返回首页'/)
    expect(enLocaleSource).toMatch(/goHome:\s*'Go to homepage'/)
  })
})
