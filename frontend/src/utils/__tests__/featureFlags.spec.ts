import { beforeEach, describe, expect, it, vi } from 'vitest'

const cachedPublicSettings = vi.hoisted(() => ({ value: undefined as Record<string, unknown> | undefined }))

vi.mock('@/stores/app', () => ({
  useAppStore: () => ({
    cachedPublicSettings: cachedPublicSettings.value,
  }),
}))

describe('feature flag registry', () => {
  beforeEach(() => {
    cachedPublicSettings.value = undefined
  })

  it('treats image generation as opt-in', async () => {
    const { FeatureFlags, isFeatureFlagEnabled } = await import('../featureFlags')

    cachedPublicSettings.value = undefined
    expect(isFeatureFlagEnabled(FeatureFlags.imageGeneration)).toBe(false)

    cachedPublicSettings.value = { image_generation_enabled: false }
    expect(isFeatureFlagEnabled(FeatureFlags.imageGeneration)).toBe(false)

    cachedPublicSettings.value = { image_generation_enabled: true }
    expect(isFeatureFlagEnabled(FeatureFlags.imageGeneration)).toBe(true)
  })

  it('treats chat completion as opt-in', async () => {
    const { FeatureFlags, isFeatureFlagEnabled } = await import('../featureFlags')

    cachedPublicSettings.value = undefined
    expect(isFeatureFlagEnabled(FeatureFlags.chatCompletion)).toBe(false)

    cachedPublicSettings.value = { chat_completion_enabled: false }
    expect(isFeatureFlagEnabled(FeatureFlags.chatCompletion)).toBe(false)

    cachedPublicSettings.value = { chat_completion_enabled: true }
    expect(isFeatureFlagEnabled(FeatureFlags.chatCompletion)).toBe(true)
  })
})
