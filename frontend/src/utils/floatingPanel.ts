export interface FloatingPanelPosition {
  top: number | null
  bottom: number | null
  left: number
  width: number
  maxHeight: number
}

export interface FloatingPanelOptions {
  viewportPadding?: number
  gap?: number
  maxWidth?: number
  maxHeightRatio?: number
  mobileBreakpoint?: number
  minComfortableHeight?: number
}

/**
 * 计算挂载到 body 的浮层位置，避免触发按钮靠近视口边缘时浮层被挤到屏幕外。
 */
export const getFloatingPanelPosition = (
  triggerRect: Pick<DOMRect, 'top' | 'right' | 'bottom'>,
  viewportWidth: number,
  viewportHeight: number,
  options: FloatingPanelOptions = {}
): FloatingPanelPosition => {
  const viewportPadding = options.viewportPadding ?? 16
  const gap = options.gap ?? 8
  const maxWidth = options.maxWidth ?? 320
  const maxHeightRatio = options.maxHeightRatio ?? 0.7
  const mobileBreakpoint = options.mobileBreakpoint ?? 768
  const minComfortableHeight = options.minComfortableHeight ?? 240

  const availableWidth = Math.max(0, viewportWidth - viewportPadding * 2)
  const width = Math.min(maxWidth, availableWidth)
  const left = viewportWidth < mobileBreakpoint
    ? viewportPadding
    : Math.max(
        viewportPadding,
        Math.min(triggerRect.right - width, viewportWidth - width - viewportPadding)
      )

  const preferredMaxHeight = Math.max(0, Math.floor(viewportHeight * maxHeightRatio))
  const spaceBelow = Math.max(0, viewportHeight - triggerRect.bottom - gap - viewportPadding)
  const spaceAbove = Math.max(0, triggerRect.top - gap - viewportPadding)
  const openAbove = spaceBelow < Math.min(minComfortableHeight, preferredMaxHeight) && spaceAbove > spaceBelow
  const maxHeight = Math.min(preferredMaxHeight, openAbove ? spaceAbove : spaceBelow)

  return {
    top: openAbove ? null : triggerRect.bottom + gap,
    bottom: openAbove ? viewportHeight - triggerRect.top + gap : null,
    left,
    width,
    maxHeight
  }
}

export type AnchoredTooltipPlacement = 'right' | 'left' | 'bottom' | 'top'

export interface AnchoredTooltipPosition {
  top: number
  left: number
  placement: AnchoredTooltipPlacement
  /** Distance from the start of the facing edge to the arrow center. */
  arrowOffset: number
}

export interface AnchoredTooltipOptions {
  tooltipWidth: number
  tooltipHeight: number
  gap?: number
  viewportPadding?: number
}

const clamp = (value: number, min: number, max: number): number => {
  if (max < min) return (min + max) / 2
  return Math.min(Math.max(value, min), max)
}

/**
 * Place a compact tooltip next to its trigger and keep the whole box in view.
 * Prefers the right side (current usage-table affordance), then left, then below/above.
 */
export const getAnchoredTooltipPosition = (
  triggerRect: Pick<DOMRect, 'top' | 'right' | 'bottom' | 'left' | 'width' | 'height'>,
  viewportWidth: number,
  viewportHeight: number,
  options: AnchoredTooltipOptions
): AnchoredTooltipPosition => {
  const gap = options.gap ?? 8
  const pad = options.viewportPadding ?? 8
  const width = Math.max(0, options.tooltipWidth)
  const height = Math.max(0, options.tooltipHeight)

  const spaceRight = viewportWidth - triggerRect.right - gap - pad
  const spaceLeft = triggerRect.left - gap - pad
  const spaceBelow = viewportHeight - triggerRect.bottom - gap - pad
  const spaceAbove = triggerRect.top - gap - pad

  let placement: AnchoredTooltipPlacement
  if (spaceRight >= width) {
    placement = 'right'
  } else if (spaceLeft >= width) {
    placement = 'left'
  } else if (spaceBelow >= height || spaceBelow >= spaceAbove) {
    placement = 'bottom'
  } else {
    placement = 'top'
  }

  const triggerCenterX = triggerRect.left + triggerRect.width / 2
  const triggerCenterY = triggerRect.top + triggerRect.height / 2

  let left: number
  let top: number
  if (placement === 'right') {
    left = triggerRect.right + gap
    top = triggerCenterY - height / 2
  } else if (placement === 'left') {
    left = triggerRect.left - gap - width
    top = triggerCenterY - height / 2
  } else if (placement === 'bottom') {
    left = triggerCenterX - width / 2
    top = triggerRect.bottom + gap
  } else {
    left = triggerCenterX - width / 2
    top = triggerRect.top - gap - height
  }

  const maxLeft = Math.max(pad, viewportWidth - width - pad)
  const maxTop = Math.max(pad, viewportHeight - height - pad)
  left = clamp(left, pad, maxLeft)
  top = clamp(top, pad, maxTop)

  const arrowOffset = placement === 'right' || placement === 'left'
    ? clamp(triggerCenterY - top, 12, height - 12)
    : clamp(triggerCenterX - left, 12, width - 12)

  return { top, left, placement, arrowOffset }
}
