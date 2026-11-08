import { describe, expect, it } from 'vitest'
import {
  DEFAULT_WATERMARK,
  MAX_WATERMARK_TILES,
  WATERMARK_ALPHA_MAX,
  WATERMARK_EDGE_RATIO,
  WATERMARK_PLACEMENTS,
  computeWatermarkStamp,
  computeWatermarkTiles,
  resolveFontSize,
  resolveStampFontSize,
  resolveSteps,
  shouldApplyWatermark,
} from '../aiImageWatermark'

/** 按行分组，方便观察「隔行错开」的规律。 */
function rowsOf(tiles: Array<{ x: number; y: number }>) {
  const byY = new Map<number, number[]>()
  for (const tile of tiles) {
    const list = byY.get(tile.y) || []
    list.push(tile.x)
    byY.set(tile.y, list)
  }
  return [...byY.entries()]
    .sort((a, b) => a[0] - b[0])
    .map(([y, xs]) => ({ y, xs: xs.sort((a, b) => a - b) }))
}

describe('水印参数：照抄桌面程序', () => {
  // 桌面程序：step_x = W // 3，step_y = H // 4
  it('间距按图片尺寸算：宽 / 3、高 / 4', () => {
    expect(resolveSteps(1200, 1600, 0, 0)).toEqual({ stepX: 400, stepY: 400 })
    expect(resolveSteps(900, 1200, 0, 0)).toEqual({ stepX: 300, stepY: 300 })
    expect(resolveSteps(1024, 1536, 0, 0)).toEqual({ stepX: 341, stepY: 384 })
  })

  it('用户显式指定间距时优先用用户的', () => {
    expect(resolveSteps(1200, 1600, 250, 180)).toEqual({ stepX: 250, stepY: 180 })
  })

  // 桌面程序：max(14, W // 30)
  it('字号按宽度算，且有 14 的下限', () => {
    expect(resolveFontSize(1200, 0)).toBe(40)
    expect(resolveFontSize(300, 0)).toBe(14)
    expect(resolveFontSize(100, 0)).toBe(14)
  })

  it('用户显式指定字号时优先用用户的', () => {
    expect(resolveFontSize(1200, 24)).toBe(24)
  })

  /**
   * 2026-10-22 主人要求「水印不能默认为熊管家AI，可以没有」。
   * 所以默认文字从写死的品牌名改成了空串 —— 这条测试就是防止有人「顺手改回去」。
   */
  it('默认没有水印文字（不再替用户署名熊管家AI）', () => {
    expect(DEFAULT_WATERMARK.text).toBe('')
    expect(DEFAULT_WATERMARK.alpha).toBe(30)
    expect(DEFAULT_WATERMARK.enabled).toBe(false)
  })

  it('开关开着但文字是空的，等于没有水印', () => {
    // 只看开关会让「勾上了却没填字」的按钮亮着但点下去没反应。
    expect(shouldApplyWatermark({ ...DEFAULT_WATERMARK, enabled: true, text: '' })).toBe(false)
    expect(shouldApplyWatermark({ ...DEFAULT_WATERMARK, enabled: true, text: '   ' })).toBe(false)
    expect(shouldApplyWatermark({ ...DEFAULT_WATERMARK, enabled: false, text: '我的店' })).toBe(false)
    expect(shouldApplyWatermark({ ...DEFAULT_WATERMARK, enabled: true, text: '我的店' })).toBe(true)
  })
})

describe('水印平铺：边界情况', () => {
  it('尺寸为 0 时没有水印，也不会卡住', () => {
    expect(computeWatermarkTiles(0, 0, DEFAULT_WATERMARK).tiles).toEqual([])
    expect(computeWatermarkTiles(0, 800, DEFAULT_WATERMARK).tiles).toEqual([])
    expect(computeWatermarkTiles(800, 0, DEFAULT_WATERMARK).tiles).toEqual([])
  })

  it('尺寸是非法数字时当作没有水印', () => {
    expect(computeWatermarkTiles(Number.NaN, 800, DEFAULT_WATERMARK).tiles).toEqual([])
    expect(computeWatermarkTiles(-100, 800, DEFAULT_WATERMARK).tiles).toEqual([])
  })

  it('间距小到离谱时会被上限截断，而不是画出几十万个', () => {
    const result = computeWatermarkTiles(4000, 4000, { ...DEFAULT_WATERMARK, stepX: 1, stepY: 1 })
    expect(result.truncated).toBe(true)
    expect(result.tiles.length).toBeLessThanOrEqual(MAX_WATERMARK_TILES)
  })
})

describe('水印平铺：隔行错开半格（桌面程序的实际行为）', () => {
  it('1200×1600 是 3 列 × 4 行，共 12 个水印', () => {
    const result = computeWatermarkTiles(1200, 1600, DEFAULT_WATERMARK)
    expect(result.tiles).toHaveLength(12)

    const rows = rowsOf(result.tiles)
    expect(rows).toHaveLength(4)
    expect(rows[0].xs).toHaveLength(3)
  })

  it('第 1、3 行从 0 开始，第 2、4 行错开半格', () => {
    const result = computeWatermarkTiles(1200, 1600, DEFAULT_WATERMARK)
    const rows = rowsOf(result.tiles)

    // stepX = 400，半格 = 200
    expect(rows[0].xs).toEqual([0, 400, 800])
    expect(rows[1].xs).toEqual([200, 600, 1000])
    expect(rows[2].xs).toEqual([0, 400, 800])
    expect(rows[3].xs).toEqual([200, 600, 1000])
  })

  it('纵向间距等于 stepY', () => {
    const result = computeWatermarkTiles(1200, 1600, DEFAULT_WATERMARK)
    const rows = rowsOf(result.tiles)
    expect(rows.map((row) => row.y)).toEqual([0, 400, 800, 1200])
  })

  // 桌面程序里没有 rotate / transpose，水印是正的。
  it('水印是正交的，没有旋转（坐标里不出现非整数偏移）', () => {
    const result = computeWatermarkTiles(1200, 1600, DEFAULT_WATERMARK)
    expect(result.tiles.every((tile) => Number.isInteger(tile.x) && Number.isInteger(tile.y))).toBe(true)
  })

  it('水印不超出左边界和上边界', () => {
    const result = computeWatermarkTiles(1200, 1600, DEFAULT_WATERMARK)
    expect(Math.min(...result.tiles.map((t) => t.x))).toBeGreaterThanOrEqual(0)
    expect(Math.min(...result.tiles.map((t) => t.y))).toBeGreaterThanOrEqual(0)
  })

  it('间距越大水印越少', () => {
    const dense = computeWatermarkTiles(1000, 1000, { ...DEFAULT_WATERMARK, stepX: 50, stepY: 50 })
    const sparse = computeWatermarkTiles(1000, 1000, { ...DEFAULT_WATERMARK, stepX: 300, stepY: 300 })
    expect(sparse.tiles.length).toBeLessThan(dense.tiles.length)
  })

  it('透明度上限是桌面程序的 100', () => {
    expect(WATERMARK_ALPHA_MAX).toBe(100)
  })
})


/**
 * 【2026-10-24 主人要求】「水印的放置位置是固定的还是怎么弄，这个需要不影响图片里文字的展示」。
 *
 * 原来照抄桌面程序是全图平铺，一定压在商品图里的俄语文案上（主人截图里那条就压在底部文字上）。
 * 现在默认改成**单个印章 + 可选位置**，默认右下角。
 */
describe('水印位置：单个印章（默认右下角）', () => {
  const W = 1200
  const H = 1600
  const margin = Math.trunc(W * WATERMARK_EDGE_RATIO)

  it('默认是右下角，不是平铺', () => {
    expect(DEFAULT_WATERMARK.placement).toBe('bottomRight')
  })

  it('四个角各自贴边，边距是宽度的 2.5%', () => {
    expect(computeWatermarkStamp(W, H, 'topLeft')).toEqual({
      x: margin,
      y: margin,
      align: 'left',
      baseline: 'top',
    })
    expect(computeWatermarkStamp(W, H, 'topRight')).toEqual({
      x: W - margin,
      y: margin,
      align: 'right',
      baseline: 'top',
    })
    expect(computeWatermarkStamp(W, H, 'bottomLeft')).toEqual({
      x: margin,
      y: H - margin,
      align: 'left',
      baseline: 'bottom',
    })
    expect(computeWatermarkStamp(W, H, 'bottomRight')).toEqual({
      x: W - margin,
      y: H - margin,
      align: 'right',
      baseline: 'bottom',
    })
  })

  it('底部正中是画面中线，且贴底', () => {
    expect(computeWatermarkStamp(W, H, 'bottomCenter')).toEqual({
      x: W / 2,
      y: H - margin,
      align: 'center',
      baseline: 'bottom',
    })
  })

  it('四个角 + 底部居中 + 平铺 都在可选列表里', () => {
    expect(WATERMARK_PLACEMENTS).toHaveLength(6)
    expect(WATERMARK_PLACEMENTS).toContain('bottomRight')
    expect(WATERMARK_PLACEMENTS).toContain('bottomCenter')
    expect(WATERMARK_PLACEMENTS).toContain('tile')
  })

  it('尺寸非法时不会算出 NaN', () => {
    const stamp = computeWatermarkStamp(0, 0, 'bottomRight')
    expect(Number.isFinite(stamp.x)).toBe(true)
    expect(Number.isFinite(stamp.y)).toBe(true)
  })

  it('单个印章的字号比平铺小一档（避免压住正文）', () => {
    // 平铺：W/30 = 40；单章：W/45 = 26
    expect(resolveFontSize(W, 0)).toBe(40)
    expect(resolveStampFontSize(W, 0)).toBe(26)
    expect(resolveStampFontSize(W, 0)).toBeLessThan(resolveFontSize(W, 0))
  })

  it('用户自己填了字号就听用户的', () => {
    expect(resolveStampFontSize(W, 88)).toBe(88)
  })

  it('小图上也不会小到看不见', () => {
    expect(resolveStampFontSize(200, 0)).toBeGreaterThanOrEqual(12)
  })
})