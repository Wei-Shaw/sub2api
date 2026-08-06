import { describe, expect, it } from 'vitest'
import { getFloatingActionMenuPosition, getFloatingPanelPosition } from '@/utils/floatingPanel'

describe('getFloatingPanelPosition', () => {
  it('移动端使用视口安全边距，不再从靠左按钮向屏幕外展开', () => {
    const position = getFloatingPanelPosition(
      { top: 160, right: 148, bottom: 200 },
      393,
      844
    )

    expect(position).toMatchObject({
      top: 208,
      bottom: null,
      left: 16,
      width: 320
    })
    expect(position.left + position.width).toBeLessThanOrEqual(393 - 16)
  })

  it('桌面端与按钮右侧对齐', () => {
    const position = getFloatingPanelPosition(
      { top: 100, right: 1000, bottom: 140 },
      1280,
      900
    )

    expect(position.left).toBe(680)
    expect(position.width).toBe(320)
  })

  it('按钮下方空间不足时改为向上展开', () => {
    const position = getFloatingPanelPosition(
      { top: 700, right: 1000, bottom: 740 },
      1280,
      800
    )

    expect(position.top).toBeNull()
    expect(position.bottom).toBe(108)
    expect(position.maxHeight).toBe(560)
  })

  it('行内操作菜单在末行向上展开并保持视口安全边距', () => {
    const position = getFloatingActionMenuPosition(
      { top: 700, right: 1200, bottom: 740 },
      1280,
      800,
      192
    )

    expect(position).toEqual({
      top: null,
      bottom: 104,
      left: 1008,
      width: 192,
      maxHeight: 640
    })
  })

  it('行内操作菜单在上下空间都不足时限制高度供内部滚动', () => {
    const position = getFloatingActionMenuPosition(
      { top: 100, right: 172, bottom: 140 },
      180,
      240,
      208
    )

    expect(position).toEqual({
      top: 144,
      bottom: null,
      left: 8,
      width: 164,
      maxHeight: 88
    })
    expect(position.top! + position.maxHeight).toBeLessThanOrEqual(240 - 8)
  })
})
