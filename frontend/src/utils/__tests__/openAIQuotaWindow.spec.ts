import { describe, expect, it } from 'vitest'
import { openAIWindowLabel } from '../openAIQuotaWindow'

describe('OpenAI quota window duration', () => {
  it.each([
    [43200, '30d'], [10080, '7d'], [300, '5h'], [90, '90m'],
    [undefined, '7d'], [0, '7d'], [-1, '7d'], [NaN, '7d'], [Infinity, '7d'], [600000, '7d']
  ])('renders %s minutes as %s', (minutes, expected) => {
    expect(openAIWindowLabel(minutes, '7d')).toBe(expected)
  })
})
