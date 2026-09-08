import { describe, expect, it } from 'vitest'
import { imageParameterLabel } from './parameterLabels'

describe('imageParameterLabel', () => {
  it('maps supported image parameters to clear Chinese labels', () => {
    expect(imageParameterLabel('quality', 'auto')).toBe('自动画质')
    expect(imageParameterLabel('quality', 'low')).toBe('低画质')
    expect(imageParameterLabel('quality', 'medium')).toBe('标准画质')
    expect(imageParameterLabel('quality', 'high')).toBe('高画质')
    expect(imageParameterLabel('background', 'auto')).toBe('自动背景')
    expect(imageParameterLabel('background', 'transparent')).toBe('透明背景')
    expect(imageParameterLabel('background', 'opaque')).toBe('不透明背景')
    expect(imageParameterLabel('input_fidelity', 'low')).toBe('低保真')
    expect(imageParameterLabel('input_fidelity', 'high')).toBe('高保真')
  })

  it('preserves unknown values from future capability definitions', () => {
    expect(imageParameterLabel('quality', 'future')).toBe('future')
  })
})
