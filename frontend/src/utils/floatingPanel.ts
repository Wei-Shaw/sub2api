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

/**
 * 计算提示浮层（挂载到 body、position: fixed）的左上角坐标，保证浮层留在视口内。
 * 主轴先放首选侧（上方或右侧），放不下就翻到对侧，两侧都放不下（手机窄屏）就贴视口边缘；
 * 交叉轴先与触发元素居中对齐，再夹在视口内。
 */
export const getTooltipPosition = (
  anchor: Pick<DOMRect, 'top' | 'right' | 'bottom' | 'left'>,
  box: { width: number; height: number },
  viewportWidth: number,
  viewportHeight: number,
  side: 'top' | 'right' = 'top',
  gap = 8
): { left: number; top: number } => {
  const clamp = (start: number, size: number, viewport: number) =>
    Math.max(gap, Math.min(start, viewport - gap - size))
  const fits = (start: number, size: number, viewport: number) =>
    start >= gap && start + size <= viewport - gap
  const flip = (preferred: number, fallback: number, size: number, viewport: number) => {
    if (fits(preferred, size, viewport)) return preferred
    if (fits(fallback, size, viewport)) return fallback
    return clamp(preferred, size, viewport)
  }

  if (side === 'right') {
    return {
      left: flip(anchor.right + gap, anchor.left - gap - box.width, box.width, viewportWidth),
      top: clamp((anchor.top + anchor.bottom - box.height) / 2, box.height, viewportHeight)
    }
  }
  return {
    left: clamp((anchor.left + anchor.right - box.width) / 2, box.width, viewportWidth),
    top: flip(anchor.top - gap - box.height, anchor.bottom + gap, box.height, viewportHeight)
  }
}
