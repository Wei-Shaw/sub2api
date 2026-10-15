/**
 * 水印：把文字平铺到图片上（桌面程序的「水印」功能）。
 *
 * 拆成两层是有意的：
 *   - computeWatermarkTiles 是**纯数学**，不碰 canvas，可以在单元测试里验证排布；
 *   - drawWatermark 才真的往 canvas 上画，jsdom 里没有 canvas，测不了。
 * 这样「平铺得对不对」是可验证的，而不是只能靠肉眼看。
 *
 * 参数全部照抄桌面程序（已反汇编确认），不要凭感觉改：
 *   - 间距是按图片尺寸算的：stepX = W/3、stepY = H/4，不是固定像素
 *   - 隔行错开半格（跑砖式），**没有旋转**
 *   - 字号 = max(14, W/30)
 *   - 输出固定 JPEG，质量 92
 */

export interface WatermarkOptions {
  /** 关掉时直接返回原图，不做任何处理。 */
  enabled: boolean
  text: string
  /** 5-100。越大越**不透明**（桌面程序的滑块标签写反了，这里按实际行为）。 */
  alpha: number
  /** 0 表示按 max(14, W/30) 自动算。 */
  fontSize: number
  color: string
  /** 0 表示按 W/3 自动算。 */
  stepX: number
  /** 0 表示按 H/4 自动算。 */
  stepY: number
}

/** 桌面程序里的默认值。 */
export const DEFAULT_WATERMARK: WatermarkOptions = {
  enabled: false,
  text: '熊管家AI',
  alpha: 30,
  fontSize: 0,
  color: '#ffffff',
  stepX: 0,
  stepY: 0,
}

/** 桌面程序的滑块范围。 */
export const WATERMARK_ALPHA_MIN = 5
export const WATERMARK_ALPHA_MAX = 100

/** 输出 JPEG 质量，和桌面程序的 92 对齐。 */
export const WATERMARK_JPEG_QUALITY = 0.92

/** 水印数量上限，防止用户把间距设得过小把浏览器卡死。 */
export const MAX_WATERMARK_TILES = 4000

/** 按图片宽度算字号：桌面程序是 max(14, W // 30)。 */
export function resolveFontSize(width: number, configured: number): number {
  if (Number.isFinite(configured) && configured > 0) return configured
  return Math.max(14, Math.trunc(width / 30))
}

/** 按图片尺寸算间距：桌面程序是 W // 3 和 H // 4。 */
export function resolveSteps(
  width: number,
  height: number,
  stepX: number,
  stepY: number,
): { stepX: number; stepY: number } {
  return {
    stepX: Number.isFinite(stepX) && stepX > 0 ? stepX : Math.max(1, Math.trunc(width / 3)),
    stepY: Number.isFinite(stepY) && stepY > 0 ? stepY : Math.max(1, Math.trunc(height / 4)),
  }
}

export interface WatermarkTile {
  x: number
  y: number
}

export interface WatermarkLayout {
  tiles: WatermarkTile[]
  /** 是否因为超过上限被截断（界面可以据此提示用户把间距调大）。 */
  truncated: boolean
}

/**
 * 计算水印平铺位置。
 *
 * 桌面程序的做法（已确认）：正交网格 + 奇数行向右错开半格。
 * 例：1200×1600 → stepX=400、stepY=400 → 横向 3 列、纵向 4 行，共 12 个水印，
 * 第 2、4 行整体右移 200px。
 *
 * 注意：**桌面程序没有旋转**（虽然它的设置窗文案写着「斜向满屏」，
 * 那是它自己的文案与实现不一致）。这里按实现来。
 */
export function computeWatermarkTiles(
  width: number,
  height: number,
  options: Pick<WatermarkOptions, 'stepX' | 'stepY'>,
): WatermarkLayout {
  const safeWidth = Number.isFinite(width) && width > 0 ? Math.floor(width) : 0
  const safeHeight = Number.isFinite(height) && height > 0 ? Math.floor(height) : 0
  if (!safeWidth || !safeHeight) return { tiles: [], truncated: false }

  const steps = resolveSteps(safeWidth, safeHeight, options.stepX, options.stepY)
  const { stepX, stepY } = steps

  const tiles: WatermarkTile[] = []
  let truncated = false
  let rowIndex = 0

  for (let y = 0; y < safeHeight; y += stepY) {
    // 奇数行（第 2、4…行）向右错开半格。
    const offset = (rowIndex % 2) * Math.trunc(stepX / 2)
    for (let x = 0; x < safeWidth; x += stepX) {
      if (tiles.length >= MAX_WATERMARK_TILES) {
        truncated = true
        return { tiles, truncated }
      }
      tiles.push({ x: x + offset, y })
    }
    rowIndex += 1
  }

  return { tiles, truncated }
}

/** 浏览器里可用的中文字体：桌面程序用的 msyh.ttc 就是微软雅黑。 */
export const WATERMARK_FONT_STACK = '"Microsoft YaHei", "PingFang SC", "Noto Sans SC", sans-serif'

/** 把水印画到已经准备好尺寸的 canvas 2d 上下文上。 */
export function drawWatermark(
  ctx: CanvasRenderingContext2D,
  width: number,
  height: number,
  options: WatermarkOptions,
): WatermarkLayout {
  const layout = computeWatermarkTiles(width, height, options)
  if (!layout.tiles.length || !options.text.trim()) return layout

  const fontSize = resolveFontSize(width, options.fontSize)

  ctx.save()
  ctx.font = `${fontSize}px ${WATERMARK_FONT_STACK}`
  ctx.textAlign = 'left'
  ctx.textBaseline = 'top'
  // alpha 是 5-100，canvas 要 0-1。
  const alpha = Math.min(WATERMARK_ALPHA_MAX, Math.max(0, options.alpha)) / 100
  ctx.globalAlpha = alpha

  for (const tile of layout.tiles) {
    // 桌面程序是纯白无描边，但那样压在浅色商品上完全看不见。
    // 这里加一圈淡描边保证任何底色都认得出——属于刻意的改进，已记录在案。
    ctx.lineWidth = Math.max(1, fontSize / 7)
    ctx.strokeStyle = 'rgba(0, 0, 0, 0.35)'
    ctx.strokeText(options.text, tile.x, tile.y)
    ctx.fillStyle = options.color
    ctx.fillText(options.text, tile.x, tile.y)
  }

  ctx.restore()
  return layout
}

/**
 * 给一张图片加水印，返回新的 Blob。
 *
 * 为什么输出 JPEG：和桌面程序一致（RGB / JPEG quality 92）。
 * 注意 JPEG 不支持透明，原图若有透明区域会被填成白底。
 */
export async function watermarkImageBlob(
  source: Blob,
  options: WatermarkOptions,
): Promise<Blob> {
  if (!options.enabled || !options.text.trim()) return source

  const bitmap = await createImageBitmap(source)
  try {
    const canvas = document.createElement('canvas')
    canvas.width = bitmap.width
    canvas.height = bitmap.height
    const ctx = canvas.getContext('2d')
    if (!ctx) throw new Error('浏览器不支持 canvas 2d，无法加水印')

    // 先铺白底：JPEG 没有透明通道，不铺的话透明区域会变黑。
    ctx.fillStyle = '#ffffff'
    ctx.fillRect(0, 0, canvas.width, canvas.height)
    ctx.drawImage(bitmap, 0, 0)
    drawWatermark(ctx, bitmap.width, bitmap.height, options)

    const output = await new Promise<Blob | null>((resolve) =>
      canvas.toBlob(resolve, 'image/jpeg', WATERMARK_JPEG_QUALITY),
    )
    if (!output) throw new Error('导出带水印的图片失败')
    return output
  } finally {
    // createImageBitmap 占的是显存，及时释放。
    bitmap.close?.()
  }
}
