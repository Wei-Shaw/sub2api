import { describe, expect, it } from 'vitest'

import {
  createVideoModelPricesForm,
  serializeVideoModelPrices,
  temporaryVideoModelKeyPrefix,
  videoModelPriceFamilyRows
} from '../groupsVideoModelPricing'

describe('video model pricing form', () => {
  it('starts empty so operators can add arbitrary model families', () => {
    const form = createVideoModelPricesForm()

    expect(videoModelPriceFamilyRows(form)).toEqual([])
  })

  it('keeps newly added model rows visually blank and skips unnamed rows', () => {
    const form = {
      [`${temporaryVideoModelKeyPrefix}1`]: { '480p': 0.2 }
    }

    expect(videoModelPriceFamilyRows(form)).toEqual([
      { key: `${temporaryVideoModelKeyPrefix}1`, label: '' }
    ])
    expect(serializeVideoModelPrices(form)).toEqual({})
  })

  it('serializes only finite non-negative prices and preserves future families', () => {
    const form = createVideoModelPricesForm({
      'grok-imagine-video-2': { '1080p': 0.4 }
    })
    form['grok-imagine-video'] = { '480p': 0.05, '720p': '', '1080p': -1 }

    expect(serializeVideoModelPrices(form)).toEqual({
      'grok-imagine-video': { '480p': 0.05 },
      'grok-imagine-video-2': { '1080p': 0.4 }
    })
  })

  it('round-trips unknown model families so editing does not discard them', () => {
    const form = createVideoModelPricesForm({
      'grok-imagine-video-2': { '480p': 0.2 }
    })

    expect(videoModelPriceFamilyRows(form).map(({ key }) => key)).toContain(
      'grok-imagine-video-2'
    )
    expect(serializeVideoModelPrices(form)).toMatchObject({
      'grok-imagine-video-2': { '480p': 0.2 }
    })
  })
})
