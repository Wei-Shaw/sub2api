import { describe, expect, it } from 'vitest'
import {
  DEFAULT_COMPOSITE,
  MAX_COMPOSITE_IMAGES,
  computeCompositeLayout,
} from '../aiImageComposite'

/** 造一个「宽/高」尺寸对象。 */
const size = (width: number, height: number) => ({ width, height })

describe('拼接排布：照抄桌面程序', () => {
  it('默认统一缩放到高 800、间隔 20', () => {
    expect(DEFAULT_COMPOSITE.height).toBe(800)
    expect(DEFAULT_COMPOSITE.gap).toBe(20)
    expect(DEFAULT_COMPOSITE.background).toBe('#ffffff')
  })

  it('0 张时画布是空的', () => {
    expect(computeCompositeLayout([]).cells).toEqual([])
    expect(computeCompositeLayout([]).width).toBe(0)
  })

  it('1 张就是它自己缩到高 800', () => {
    const layout = computeCompositeLayout([size(600, 800)])
    expect(layout.cells).toHaveLength(1)
    expect(layout.height).toBe(800)
    expect(layout.cells[0]).toMatchObject({ x: 0, y: 0, width: 600, height: 800 })
  })

  it('2 张横排，中间留 20px', () => {
    const layout = computeCompositeLayout([size(600, 800), size(600, 800)])
    expect(layout.cells).toHaveLength(2)
    expect(layout.cells[0].x).toBe(0)
    expect(layout.cells[1].x).toBe(620)
    // 总宽 = 600 + 20 + 600
    expect(layout.width).toBe(1220)
  })

  // 桌面程序是「图与图之间 20px」，最后一个右边没有 20px。
  // 别写成 20 * n —— 那会多算一个间距。
  it('总宽只算 n-1 个间距', () => {
    const layout = computeCompositeLayout([size(600, 800), size(600, 800), size(600, 800)])
    expect(layout.width).toBe(600 * 3 + 20 * 2)
  })

  it('每张等比缩放到统一高度，宽按原比例算', () => {
    // 1920×1080 横图 → 高 800 → 宽 1422
    const layout = computeCompositeLayout([size(1920, 1080)])
    expect(layout.cells[0].height).toBe(800)
    expect(layout.cells[0].width).toBe(Math.round((1920 * 800) / 1080))
  })

  it('所有格子都是同一高度，且都在 y=0 对齐', () => {
    const layout = computeCompositeLayout([size(600, 800), size(1600, 900), size(400, 1000)])
    expect(layout.cells.every((cell) => cell.height === 800)).toBe(true)
    expect(layout.cells.every((cell) => cell.y === 0)).toBe(true)
  })

  it('格子依次排开，不重叠', () => {
    const layout = computeCompositeLayout([size(600, 800), size(600, 800), size(600, 800)])
    for (let index = 1; index < layout.cells.length; index += 1) {
      const previous = layout.cells[index - 1]
      expect(layout.cells[index].x).toBeGreaterThanOrEqual(previous.x + previous.width)
    }
  })

  it('最后一个格子的右边缘就是画布右边缘', () => {
    const layout = computeCompositeLayout([size(600, 800), size(600, 800)])
    const last = layout.cells[layout.cells.length - 1]
    expect(last.x + last.width).toBe(layout.width)
  })

  it('尺寸非法的图会被跳过，不会算出 NaN', () => {
    const layout = computeCompositeLayout([size(600, 800), size(0, 0), size(-5, 100)])
    expect(layout.cells).toHaveLength(1)
    expect(Number.isFinite(layout.width)).toBe(true)
  })

  // 桌面程序没限制张数，但横排 10 张 3:4 竖图会到 6180px 宽，生图接口普遍吃不下。
  it('超过张数上限时只取前几张，画布不会无限宽', () => {
    const many = Array.from({ length: 20 }, () => size(600, 800))
    const layout = computeCompositeLayout(many)
    expect(layout.cells).toHaveLength(MAX_COMPOSITE_IMAGES)
  })

  it('间距可调', () => {
    const layout = computeCompositeLayout([size(600, 800), size(600, 800)], {
      ...DEFAULT_COMPOSITE,
      gap: 50,
    })
    expect(layout.cells[1].x).toBe(650)
  })

  it('高度可调', () => {
    const layout = computeCompositeLayout([size(600, 800)], { ...DEFAULT_COMPOSITE, height: 400 })
    expect(layout.height).toBe(400)
    expect(layout.cells[0].height).toBe(400)
  })
})
