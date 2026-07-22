import { describe, expect, it REDACTED from 'vitest'
import { getFloatingPanelPosition REDACTED from '@/utils/floatingPanel'

describe('getFloatingPanelPosition', () => {
  it('移动端使用视口安全边距，不再从靠左按钮向屏幕外展开', () => {
    const position = getFloatingPanelPosition(
      { top: 160, right: 148, bottom: 200 REDACTED,
      393,
      844
    )

    expect(position).toMatchObject({
      top: 208,
      bottom: null,
      left: 16,
      width: 320
    REDACTED)
    expect(position.left + position.width).toBeLessThanOrEqual(393 - 16)
  REDACTED)

  it('桌面端与按钮右侧对齐', () => {
    const position = getFloatingPanelPosition(
      { top: 100, right: 1000, bottom: 140 REDACTED,
      1280,
      900
    )

    expect(position.left).toBe(680)
    expect(position.width).toBe(320)
  REDACTED)

  it('按钮下方空间不足时改为向上展开', () => {
    const position = getFloatingPanelPosition(
      { top: 700, right: 1000, bottom: 740 REDACTED,
      1280,
      800
    )

    expect(position.top).toBeNull()
    expect(position.bottom).toBe(108)
    expect(position.maxHeight).toBe(560)
  REDACTED)
REDACTED)
