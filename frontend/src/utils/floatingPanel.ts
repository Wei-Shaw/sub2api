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

const ACTION_MENU_VIEWPORT_PADDING = 8
const ACTION_MENU_GAP = 4
const ACTION_MENU_MAX_HEIGHT_RATIO = 0.8
const ACTION_MENU_COMFORTABLE_HEIGHT = 320

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
 * 计算行内操作菜单的位置。菜单始终限制在视口可用高度内；当任一方向
 * 都无法完整容纳菜单时，由调用方通过 maxHeight 提供内部滚动。
 */
export const getFloatingActionMenuPosition = (
  triggerRect: Pick<DOMRect, 'top' | 'right' | 'bottom'>,
  viewportWidth: number,
  viewportHeight: number,
  maxWidth: number
): FloatingPanelPosition => getFloatingPanelPosition(
  triggerRect,
  viewportWidth,
  viewportHeight,
  {
    viewportPadding: ACTION_MENU_VIEWPORT_PADDING,
    gap: ACTION_MENU_GAP,
    maxWidth,
    maxHeightRatio: ACTION_MENU_MAX_HEIGHT_RATIO,
    minComfortableHeight: ACTION_MENU_COMFORTABLE_HEIGHT
  }
)
