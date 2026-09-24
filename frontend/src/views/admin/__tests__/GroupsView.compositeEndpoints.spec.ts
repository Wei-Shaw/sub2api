import { describe, expect, it } from 'vitest'

import en from '@/i18n/locales/en'
import zh from '@/i18n/locales/zh'
import type { CompositeRouteEndpoint } from '@/types'

describe('GroupsView Composite route endpoints', () => {
  it('accepts systemone as a routable endpoint', () => {
    const endpoint: CompositeRouteEndpoint = 'systemone'
    expect(endpoint).toBe('systemone')
  })

  it('labels the systemone endpoint in both locales', () => {
    expect(en.admin.groups.compositeRoutes.endpoints.systemone).toBe('SystemOne (Jev)')
    expect(zh.admin.groups.compositeRoutes.endpoints.systemone).toBe('SystemOne (Jev)')
  })
})
