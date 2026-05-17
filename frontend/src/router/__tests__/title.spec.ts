import { readFileSync } from 'node:fs'
import { resolve } from 'node:path'

import { describe, expect, it, vi } from 'vitest'

vi.mock('@/i18n', () => ({
  i18n: {
    global: {
      t: (key: string) => key,
    },
  },
}))

import { resolveDocumentTitle } from '@/router/title'

describe('resolveDocumentTitle', () => {
  it('路由存在标题时，使用“路由标题 - 站点名”格式', () => {
    expect(resolveDocumentTitle('Usage Records', 'My Site')).toBe('Usage Records - My Site')
  })

  it('路由无标题时，回退到站点名', () => {
    expect(resolveDocumentTitle(undefined, 'My Site')).toBe('My Site')
  })

  it('站点名为空时，回退默认站点名', () => {
    expect(resolveDocumentTitle('Dashboard', '')).toBe('Dashboard - DevRouter')
    expect(resolveDocumentTitle(undefined, '   ')).toBe('DevRouter')
  })

  it('站点名变更时仅影响后续路由标题计算', () => {
    const before = resolveDocumentTitle('Admin Dashboard', 'Alpha')
    const after = resolveDocumentTitle('Admin Dashboard', 'Beta')

    expect(before).toBe('Admin Dashboard - Alpha')
    expect(after).toBe('Admin Dashboard - Beta')
  })

  it('declares secondary-development frontend routes without backend coupling', () => {
    const routerSource = readFileSync(resolve(process.cwd(), 'src/router/index.ts'), 'utf8')

    expect(routerSource).toContain("path: '/models'")
    expect(routerSource).toContain("path: '/images'")
    expect(routerSource).toContain("titleKey: 'nav.imageGeneration'")
    expect(routerSource).toContain("path: '/chat'")
    expect(routerSource).toContain("titleKey: 'nav.chatCompletion'")
    expect(routerSource).toContain("path: '/recharge-subscription'")
    expect(routerSource).toContain("path: '/profile'")
    expect(routerSource).not.toContain("path: '/docs'")
    expect(routerSource).not.toContain("name: 'UsageDocs'")
    expect(routerSource).not.toContain("backend/")
  })

  it('declares the channel status route as public for landing-page links', () => {
    const routerSource = readFileSync(resolve(process.cwd(), 'src/router/index.ts'), 'utf8')

    expect(routerSource).toMatch(/path: '\/monitor'[\s\S]*requiresAuth: false/)
    expect(routerSource).toMatch(/path: '\/monitor'[\s\S]*standalone: true/)
  })
})
