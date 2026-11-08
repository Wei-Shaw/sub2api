/**
 * Pure X-axis zoom/pan for channel-monitor pulse matrix and line chart.
 * No chartjs-plugin-zoom dependency — state is span + offset fractions.
 */

export interface ZoomState {
  /** Visible fraction of the full series (1 = full range). */
  span: number
  /** Start of the visible window as a fraction of the full domain [0, 1 - span]. */
  offset: number
}

export const DEFAULT_ZOOM: ZoomState = { span: 1, offset: 0 }
export const MIN_ZOOM_SPAN = 0.12

const ZOOM_STEP = 0.12

function clamp(value: number, min: number, max: number): number {
  return Math.min(max, Math.max(min, value))
}

export function resetZoom(): ZoomState {
  return { ...DEFAULT_ZOOM }
}

export function isZoomed(state: ZoomState): boolean {
  return state.span < 0.999 || state.offset > 0.001
}

/** Normalize offset so the visible window stays within [0, 1]. */
export function clampZoom(state: ZoomState): ZoomState {
  const span = clamp(state.span, MIN_ZOOM_SPAN, 1)
  const maxOffset = Math.max(0, 1 - span)
  return {
    span,
    offset: clamp(state.offset, 0, maxOffset),
  }
}

/**
 * Wheel zoom around cursor xRatio (0–1 within the chart/matrix track).
 * Shift+wheel or horizontal delta pans offset instead.
 */
export function applyWheelZoom(
  state: ZoomState,
  event: Pick<WheelEvent, 'deltaY' | 'deltaX' | 'shiftKey'>,
  xRatio: number,
): ZoomState {
  const ratio = clamp(Number.isFinite(xRatio) ? xRatio : 0.5, 0, 1)
  const isPan = event.shiftKey || Math.abs(event.deltaX) > Math.abs(event.deltaY)

  if (isPan) {
    if (state.span >= 1) return clampZoom(state)
    const delta = event.shiftKey ? event.deltaY : event.deltaX
    // Positive delta → pan right (increase offset)
    const panFraction = (delta / 400) * state.span
    return clampZoom({
      span: state.span,
      offset: state.offset + panFraction,
    })
  }

  // Zoom in (deltaY < 0) / out (deltaY > 0) around cursor
  const direction = event.deltaY < 0 ? -1 : event.deltaY > 0 ? 1 : 0
  if (direction === 0) return clampZoom(state)

  const nextSpan = clamp(state.span * (1 + direction * ZOOM_STEP), MIN_ZOOM_SPAN, 1)
  if (Math.abs(nextSpan - state.span) < 1e-9) return clampZoom(state)

  // Keep the point under the cursor fixed in data space.
  // windowStart is offset (domain fraction); dataPos = windowStart + ratio * span
  const windowStart = state.offset
  const dataPos = windowStart + ratio * state.span
  const nextStart = dataPos - ratio * nextSpan

  return clampZoom({
    span: nextSpan,
    offset: nextStart,
  })
}

/**
 * Slice an array using zoom state. Cursor-centered windowing — not always "last N".
 */
export function sliceByZoom<T>(items: readonly T[], state: ZoomState): T[] {
  if (!items.length) return []
  const zoom = clampZoom(state)
  if (zoom.span >= 0.999) return [...items]

  const count = Math.max(1, Math.ceil(items.length * zoom.span))
  const maxStart = Math.max(0, items.length - count)
  // offset is window start in [0, 1-span] domain → map to index
  const start = Math.min(maxStart, Math.max(0, Math.round(zoom.offset * items.length)))
  return items.slice(start, start + count)
}

/** Map a client X position within an element to 0–1 ratio. */
export function clientXRatio(clientX: number, element: HTMLElement | null | undefined): number {
  if (!element) return 0.5
  const rect = element.getBoundingClientRect()
  if (rect.width <= 0) return 0.5
  return clamp((clientX - rect.left) / rect.width, 0, 1)
}
