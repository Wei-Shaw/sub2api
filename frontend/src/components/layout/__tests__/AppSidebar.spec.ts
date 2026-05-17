import { readFileSync } from 'node:fs'
import { dirname, resolve } from 'node:path'
import { fileURLToPath } from 'node:url'

import { describe, expect, it } from 'vitest'

const componentPath = resolve(dirname(fileURLToPath(import.meta.url)), '../AppSidebar.vue')
const componentSource = readFileSync(componentPath, 'utf8')
const stylePath = resolve(dirname(fileURLToPath(import.meta.url)), '../../../style.css')
const styleSource = readFileSync(stylePath, 'utf8')

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
  it('does not clip header content', () => {
    const sidebarHeaderBlockMatch = styleSource.match(/\.sidebar-header\s*\{[\s\S]*?\n {2}\}/)
    const sidebarBrandBlockMatch = componentSource.match(/\.sidebar-brand\s*\{[\s\S]*?\n\}/)

    expect(sidebarHeaderBlockMatch).not.toBeNull()
    expect(sidebarBrandBlockMatch).not.toBeNull()
    expect(sidebarHeaderBlockMatch?.[0]).not.toContain('@apply overflow-hidden;')
    expect(sidebarBrandBlockMatch?.[0]).not.toContain('overflow: hidden;')
  })

  it('does not trigger official update checks from the sidebar', () => {
    expect(componentSource).toContain('<VersionBadge :version="siteVersion" />')
    expect(componentSource).not.toContain('fetchVersion')
    expect(componentSource).not.toContain('checkUpdates')
  })
})

describe('AppSidebar theme control', () => {
  it('does not render a dark-mode toggle in the sidebar', () => {
    expect(componentSource).not.toContain('@click="toggleTheme"')
    expect(componentSource).not.toContain("t('nav.lightMode')")
    expect(componentSource).not.toContain("t('nav.darkMode')")
    expect(componentSource).not.toContain('useThemeTransition')
    expect(componentSource).not.toContain('SunIcon')
    expect(componentSource).not.toContain('MoonIcon')
  })
})

describe('AppSidebar secondary-development navigation contract', () => {
  it('keeps profile, hides stale docs/channel-status entries, and adds unified user entries', () => {
    expect(componentSource).toContain("path: '/profile'")
    expect(componentSource).toContain("path: '/models'")
    expect(componentSource).not.toContain("path: '/monitor', label: t('nav.channelMonitor'), icon: SignalIcon, featureFlag: flagChannelMonitor")
    expect(componentSource).toContain("path: '/images', label: t('nav.imageGeneration'), icon: PhotoIcon, featureFlag: flagImageGeneration")
    expect(componentSource).toContain("path: '/chat', label: t('nav.chatCompletion'), icon: ChatIcon, featureFlag: flagChatCompletion")
    expect(componentSource.indexOf("path: '/images'")).toBeLessThan(componentSource.indexOf("path: '/chat'"))
    expect(componentSource.indexOf("path: '/chat'")).toBeLessThan(componentSource.indexOf("path: '/usage'"))
    expect(componentSource).toContain("path: '/recharge-subscription', label: t('nav.rechargeSubscription'), icon: RechargeSubscriptionIcon, featureFlag: flagPayment")
    expect(componentSource).not.toContain("path: '/docs'")
    expect(componentSource).toContain('const flagImageGeneration = makeSidebarFlag(FeatureFlags.imageGeneration)')
    expect(componentSource).toContain('const flagChatCompletion = makeSidebarFlag(FeatureFlags.chatCompletion)')
    expect(componentSource).toContain("path: '/recharge-subscription'")
    expect(componentSource).not.toContain("path: '/monitor', label: t('nav.channelStatus')")
  })

  it('uses a home-style label for the user dashboard item', () => {
    expect(componentSource).toContain("path: '/dashboard', label: t('nav.home')")
  })
  it('keeps admin channel monitor reachable in simple mode while hiding channel pricing', () => {
    expect(componentSource).toContain('function applySimpleModeFilter(items: NavItem[]): NavItem[]')
    expect(componentSource).toContain("path: '/admin/channels/pricing', label: t('nav.channelPricing'), icon: PriceTagIcon, hideInSimpleMode: true")
    expect(componentSource).toContain("path: '/admin/channels/monitor', label: t('nav.channelMonitor'), icon: SignalIcon, featureFlag: flagChannelMonitor")
  })
})
