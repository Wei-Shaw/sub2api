import { beforeEach, describe, expect, it, vi } from 'vitest'

const appStore = vi.hoisted(() => ({
  cachedPublicSettings: undefined as Record<string, unknown> | undefined,
}))

vi.mock('@/stores/app', () => ({
  useAppStore: () => appStore,
}))

import { FeatureFlags, isFeatureFlagEnabled } from '@/utils/featureFlags'

describe('model-prices feature flag', () => {
  beforeEach(() => {
    appStore.cachedPublicSettings = undefined
  })

  it('stays visible while public settings are missing from an older or loading payload', () => {
    expect(isFeatureFlagEnabled(FeatureFlags.modelPlaza)).toBe(true)
  })

  it('still respects an explicit backend disable', () => {
    appStore.cachedPublicSettings = { model_plaza_enabled: false }
    expect(isFeatureFlagEnabled(FeatureFlags.modelPlaza)).toBe(false)
  })
})
