import { beforeEach, describe, expect, it } from 'vitest'
import {
  clearCustomBackground,
  initCustomBackground,
  setCustomBackgroundEnabled,
  setCustomBackground,
  setCustomComponentEffect,
  useCustomBackground,
} from '../useCustomBackground'

describe('useCustomBackground', () => {
  beforeEach(() => {
    localStorage.clear()
    document.documentElement.removeAttribute('data-custom-background')
    document.documentElement.removeAttribute('data-custom-component-effect')
    document.documentElement.style.removeProperty('--custom-background-image')
    document.documentElement.style.removeProperty('--custom-background-opacity')
    initCustomBackground()
  })

  it('persists and applies an image with a clamped opacity', () => {
    setCustomBackground({ image: 'data:image/png;base64,abc', opacity: 1.4, componentEffect: 'transparent' })

    expect(useCustomBackground().customBackground.value).toEqual({
      image: 'data:image/png;base64,abc',
      opacity: 1,
      componentEffect: 'transparent',
      enabled: true,
    })
    expect(localStorage.getItem('custom-background')).toContain('data:image/png;base64,abc')
    expect(document.documentElement.dataset.customBackground).toBe('true')
    expect(document.documentElement.dataset.customComponentEffect).toBe('transparent')
    expect(document.documentElement.style.getPropertyValue('--custom-background-opacity')).toBe('1')
  })

  it('turns the background off without removing the saved image', async () => {
    setCustomBackground({ image: 'data:image/png;base64,abc', opacity: 0.5, componentEffect: 'frosted' })

    await setCustomBackgroundEnabled(false)
    expect(useCustomBackground().customBackground.value.image).toContain('data:image/png')
    expect(useCustomBackground().customBackground.value.enabled).toBe(false)
    expect(document.documentElement.dataset.customBackground).toBe('false')
    expect(document.documentElement.style.getPropertyValue('--custom-background-opacity')).toBe('0')

    await setCustomBackgroundEnabled(true)
    expect(useCustomBackground().customBackground.value.enabled).toBe(true)
    expect(document.documentElement.dataset.customBackground).toBe('true')
    expect(document.documentElement.style.getPropertyValue('--custom-background-opacity')).toBe('0.5')
  })

  it('restores saved settings and clears the background independently', async () => {
    localStorage.setItem(
      'custom-background',
      JSON.stringify({ image: 'data:image/jpeg;base64,xyz', opacity: 0.35, componentEffect: 'frosted' })
    )
    initCustomBackground()
    expect(useCustomBackground().customBackground.value.opacity).toBe(0.35)
    expect(document.documentElement.dataset.customBackground).toBe('true')

    await setCustomComponentEffect('transparent')
    expect(useCustomBackground().customBackground.value.componentEffect).toBe('transparent')
    expect(document.documentElement.dataset.customComponentEffect).toBe('transparent')

    clearCustomBackground()
    expect(useCustomBackground().customBackground.value.image).toBe('')
    expect(document.documentElement.dataset.customBackground).toBe('false')
    expect(document.documentElement.dataset.customComponentEffect).toBe('frosted')
    expect(useCustomBackground().customBackground.value.enabled).toBe(false)
  })
})
