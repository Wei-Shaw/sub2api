import { readFileSync } from 'node:fs'
import { dirname, resolve } from 'node:path'
import { fileURLToPath } from 'node:url'
import { describe, expect, it } from 'vitest'

const directory = dirname(fileURLToPath(import.meta.url))
const routerSource = readFileSync(resolve(directory, '../index.ts'), 'utf8')

describe('groups status public route', () => {
  it('exposes /groups-status anonymously and keeps it available in backend mode', () => {
    const routeStart = routerSource.indexOf("path: '/groups-status'")
    const routeBlock = routerSource.slice(routeStart, routerSource.indexOf('// ==================== User Routes', routeStart))

    expect(routeStart).toBeGreaterThan(-1)
    expect(routeBlock).toContain("name: 'GroupsStatus'")
    expect(routeBlock).toContain('requiresAuth: false')
    expect(routeBlock).toContain("titleKey: 'groupsStatus.title'")
    expect(routerSource).toMatch(/BACKEND_MODE_ALLOWED_PATHS = \[[^\]]*'\/groups-status'/)
  })
})
