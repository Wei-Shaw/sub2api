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

describe('AppSidebar scroll position persistence', () => {
  it('binds a template ref to the sidebar nav element', () => {
    expect(componentSource).toContain('ref="sidebarNavRef"')
    expect(componentSource).toContain('sidebar-nav')
  })

  it('declares sidebarNavRef in script setup', () => {
    expect(componentSource).toContain("const sidebarNavRef = ref<HTMLElement | null>(null)")
  })

  it('saves scroll position on beforeUnmount', () => {
    expect(componentSource).toContain('onBeforeUnmount')
    expect(componentSource).toContain('appStore.sidebarScrollTop')
    expect(componentSource).toContain('sidebarNavRef.value.scrollTop')
  })

  it('restores scroll position on mount', () => {
    expect(componentSource).toContain('onMounted')
    expect(componentSource).toContain('appStore.sidebarScrollTop')
    expect(componentSource).toContain('nextTick')
  })
})

describe('AppSidebar collapsible groups', () => {
  it('lets the user collapse a group even while a child route is active', () => {
    // The expand state must come from the user's override first, falling back
    // to the active-route heuristic only when the user has not clicked yet.
    expect(componentSource).toContain('const groupExpandOverrides = ref<Map<string, boolean>>(new Map())')
    expect(componentSource).not.toContain('expandedGroups.value.has(item.path) || isGroupActive(item)')
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

describe('AppSidebar subscription feature flag', () => {
  it('gates the My Subscriptions entry behind the subscription public-settings flag', () => {
    expect(componentSource).toContain('const flagSubscription = makeSidebarFlag(FeatureFlags.subscription)')
    expect(componentSource).toMatch(/path: '\/subscriptions'[^\n]*featureFlag: flagSubscription/)
  })

  it('also hides the admin Subscription Management entry on recharge-only sites', () => {
    expect(componentSource).toMatch(/path: '\/admin\/subscriptions'[^\n]*featureFlag: flagSubscription/)
  })

  it('derives the purchase entry label from the site billing mode', () => {
    expect(componentSource).toContain("import { resolveSiteBillingMode } from '@/utils/siteBillingMode'")
    expect(componentSource).toMatch(/case 'recharge_only':\s*return t\('nav\.recharge'\)/)
    expect(componentSource).toMatch(/case 'subscription_only':\s*return t\('nav\.subscribe'\)/)
    expect(componentSource).toMatch(/path: '\/purchase'[^\n]*label: purchaseNavLabel\.value/)
  })
})

describe('AppSidebar AI image entry', () => {
  // buildSelfNavItems 同时供给用户端主菜单与管理端「我的账户」分组，
  // 所以只在这里声明一次就能让两端落在同一个相对位置。
  const selfNavBlock = componentSource.match(
    /function buildSelfNavItems\([\s\S]*?\n\}/,
  )?.[0]

  it('declares the entry inside the shared self-nav builder', () => {
    expect(selfNavBlock).toBeDefined()
    expect(selfNavBlock).toMatch(/path: '\/ai-image'[^\n]*label: t\('nav\.aiImage'\)/)
  })

  it('places it directly below 智力检测 and directly above 我的订阅', () => {
    expect(selfNavBlock).toBeDefined()
    const block = selfNavBlock as string
    const intelligenceIndex = block.indexOf("path: '/intelligence-check'")
    const aiImageIndex = block.indexOf("path: '/ai-image'")
    const subscriptionsIndex = block.indexOf("path: '/subscriptions'")

    expect(intelligenceIndex).toBeGreaterThan(-1)
    expect(aiImageIndex).toBeGreaterThan(-1)
    expect(subscriptionsIndex).toBeGreaterThan(-1)
    expect(aiImageIndex).toBeGreaterThan(intelligenceIndex)
    expect(aiImageIndex).toBeLessThan(subscriptionsIndex)
  })

  it('is rendered by both the user nav and the admin personal nav', () => {
    expect(componentSource).toContain('finalizeNav(buildSelfNavItems(true))')
    expect(componentSource).toContain('finalizeNav(buildSelfNavItems(false))')
  })
})

describe('AppSidebar 的 AI生图 入口（2026-10-22 起对普通用户开放）', () => {
  // 主人 2026-10-22 明确要求「把AI生图这个模块变为普通用户也可以使用」。
  // 所以在它「先只给管理员」的时期过去之后，这两处限制必须都没了。
  it('AI生图 入口不再标 adminOnly（普通用户也看得到）', () => {
    const entry = componentSource.match(/path: '\/ai-image'[^\n]*/)?.[0]
    expect(entry).toBeDefined()
    expect(entry).not.toContain('adminOnly')
  })

  it('adminOnly 机制本身还留着（别的模块以后要用）', () => {
    const finalizeBlock = componentSource.match(/function finalizeNav\([\s\S]*?\n\}/)?.[0]
    expect(finalizeBlock).toBeDefined()
    expect(finalizeBlock).toContain('authStore.isAdmin')
    expect(finalizeBlock).toMatch(/adminOnly/)
    expect(componentSource).toMatch(/adminOnly\?: boolean/)
  })
})

describe('AI image route（2026-10-22 起不再要求管理员）', () => {
  const routerSource = readFileSync(
    resolve(dirname(fileURLToPath(import.meta.url)), '../../../router/index.ts'),
    'utf8',
  )

  it('/ai-image 不再要求管理员，但仍然要求登录', () => {
    const routeBlock = routerSource.match(/path: '\/ai-image',[\s\S]*?\n {2}\},/)?.[0]
    expect(routeBlock).toBeDefined()
    // 关键：去掉了管理员限制
    expect(routeBlock).not.toContain('requiresAdmin')
    // 但登录还是必须的（不然未登录也能进）
    expect(routeBlock).toContain('requiresAuth: true')
  })
})
