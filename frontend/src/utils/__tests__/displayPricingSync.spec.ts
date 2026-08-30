import { describe, expect, it, vi } from 'vitest'
import {
  notifyDisplayPricingUpdated,
  subscribeDisplayPricingUpdates,
  DISPLAY_PRICING_SYNC_STORAGE_KEY
} from '../displayPricingSync'

describe('display pricing sync', () => {
  it('immediately notifies a catalogue mounted in the current document', () => {
    const callback = vi.fn()
    const unsubscribe = subscribeDisplayPricingUpdates(callback)

    notifyDisplayPricingUpdated()

    expect(callback).toHaveBeenCalledTimes(1)
    expect(window.localStorage.getItem(DISPLAY_PRICING_SYNC_STORAGE_KEY)).toMatch(/^\d+$/)
    unsubscribe()
  })

  it('stops local notifications after unsubscribe', () => {
    const callback = vi.fn()
    const unsubscribe = subscribeDisplayPricingUpdates(callback)
    unsubscribe()

    notifyDisplayPricingUpdated()

    expect(callback).not.toHaveBeenCalled()
  })
})
