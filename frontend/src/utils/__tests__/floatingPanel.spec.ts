import { describe, expect, it } from 'vitest'
import { getAnchoredTooltipPosition, getFloatingPanelPosition } from '@/utils/floatingPanel'

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
})

describe('getAnchoredTooltipPosition', () => {
  const tooltip = { tooltipWidth: 280, tooltipHeight: 220 }

  it('keeps the default right-side placement when there is room', () => {
    const position = getAnchoredTooltipPosition(
      { top: 200, right: 180, bottom: 216, left: 164, width: 16, height: 16 },
      1280,
      800,
      tooltip
    )

    expect(position.placement).toBe('right')
    expect(position.left).toBe(188)
    expect(position.top).toBe(98)
    expect(position.left + 280).toBeLessThanOrEqual(1280 - 8)
  })

  it('flips to the left when the info icon sits on a mobile card edge', () => {
    const position = getAnchoredTooltipPosition(
      { top: 420, right: 374, bottom: 436, left: 358, width: 16, height: 16 },
      390,
      844,
      tooltip
    )

    expect(position.placement).toBe('left')
    expect(position.left).toBeGreaterThanOrEqual(8)
    expect(position.left + 280).toBeLessThanOrEqual(358 - 8)
    expect(position.top).toBeGreaterThanOrEqual(8)
    expect(position.top + 220).toBeLessThanOrEqual(844 - 8)
  })

  it('opens below and stays inside a narrow viewport when neither side fits', () => {
    const position = getAnchoredTooltipPosition(
      { top: 200, right: 308, bottom: 216, left: 292, width: 16, height: 16 },
      320,
      700,
      { tooltipWidth: 300, tooltipHeight: 180 }
    )

    expect(position.placement).toBe('bottom')
    expect(position.left).toBeGreaterThanOrEqual(8)
    expect(position.left + 300).toBeLessThanOrEqual(320 - 8)
    expect(position.top).toBe(224)
  })

  it('opens above when the trigger is near the bottom of the screen', () => {
    const position = getAnchoredTooltipPosition(
      { top: 760, right: 200, bottom: 776, left: 40, width: 16, height: 16 },
      390,
      800,
      { tooltipWidth: 360, tooltipHeight: 200 }
    )

    expect(position.placement).toBe('top')
    expect(position.top + 200).toBeLessThanOrEqual(760 - 8)
    expect(position.left).toBeGreaterThanOrEqual(8)
    expect(position.left + 360).toBeLessThanOrEqual(390 - 8)
  })

  it('clamps a tall tooltip so it does not overflow the viewport vertically', () => {
    const position = getAnchoredTooltipPosition(
      { top: 20, right: 200, bottom: 36, left: 184, width: 16, height: 16 },
      1280,
      400,
      { tooltipWidth: 240, tooltipHeight: 360 }
    )

    expect(position.top).toBe(8)
    expect(position.top + 360).toBeLessThanOrEqual(400)
    expect(position.arrowOffset).toBeGreaterThanOrEqual(12)
  })
})
