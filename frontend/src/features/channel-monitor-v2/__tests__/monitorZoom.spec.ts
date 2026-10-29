import { describe, expect, it } from 'vitest'
import {
  DEFAULT_ZOOM,
  MIN_ZOOM_SPAN,
  applyWheelZoom,
  clampZoom,
  isZoomed,
  resetZoom,
  sliceByZoom,
} from '../monitorZoom'

describe('monitorZoom', () => {
  it('resets to full span', () => {
    expect(resetZoom()).toEqual(DEFAULT_ZOOM)
    expect(isZoomed(DEFAULT_ZOOM)).toBe(false)
  })

  it('zooms in around cursor without always pinning to the end', () => {
    const mid = applyWheelZoom(DEFAULT_ZOOM, { deltaY: -100, deltaX: 0, shiftKey: false }, 0.5)
    expect(mid.span).toBeLessThan(1)
    expect(mid.span).toBeGreaterThanOrEqual(MIN_ZOOM_SPAN)
    // Center zoom keeps window roughly centered
    expect(mid.offset).toBeGreaterThan(0)
    expect(mid.offset + mid.span).toBeLessThanOrEqual(1.001)

    const left = applyWheelZoom(DEFAULT_ZOOM, { deltaY: -100, deltaX: 0, shiftKey: false }, 0)
    expect(left.offset).toBeLessThanOrEqual(0.001)

    const right = applyWheelZoom(DEFAULT_ZOOM, { deltaY: -100, deltaX: 0, shiftKey: false }, 1)
    expect(right.offset + right.span).toBeGreaterThan(0.99)
  })

  it('pans with shift+wheel', () => {
    const zoomed = clampZoom({ span: 0.4, offset: 0.1 })
    const panned = applyWheelZoom(zoomed, { deltaY: 200, deltaX: 0, shiftKey: true }, 0.5)
    expect(panned.span).toBeCloseTo(zoomed.span, 5)
    expect(panned.offset).toBeGreaterThan(zoomed.offset)
  })

  it('clamps span to MIN_ZOOM_SPAN and keeps window in range', () => {
    const tight = clampZoom({ span: 0.01, offset: 0.9 })
    expect(tight.span).toBe(MIN_ZOOM_SPAN)
    expect(tight.offset + tight.span).toBeLessThanOrEqual(1.001)
  })

  it('slices series by window, not always last N points', () => {
    const items = Array.from({ length: 100 }, (_, i) => i)
    const startWindow = sliceByZoom(items, { span: 0.2, offset: 0 })
    expect(startWindow[0]).toBe(0)
    expect(startWindow.length).toBe(20)

    const midWindow = sliceByZoom(items, { span: 0.2, offset: 0.4 })
    expect(midWindow[0]).toBe(40)
    expect(midWindow[midWindow.length - 1]).toBe(59)

    const full = sliceByZoom(items, DEFAULT_ZOOM)
    expect(full).toHaveLength(100)
  })
})
