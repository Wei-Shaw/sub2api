/**
 * 水印：把文字打到图片上。
 *
 * 拆成两层是有意的：
 *   - 计算位置的部分是**纯数学**，不碰 canvas，可以在单元测试里验证排布；
 *   - drawWatermark 才真的往 canvas 上画，jsdom 里没有 canvas，测不了。
 *
 * 【2026-10-24 主人要求】「水印的放置位置是固定的还是怎么弄，这个需要不影响图片里文字的展示」。
 * 原来照抄桌面程序是**全图平铺**（stepX=W/3、stepY=H/4），铺满整张图，
 * 一定会压在商品图里的俄语文案上（主人截图里那条就压在底部文字上）。
 * 所以现在默认改成**单个印章**，位置可选四角或底部居中；平铺保留成一种可选项。
 */

/** 水印位置：单个印章放四角 / 底部居中，或者保留原来的全图平铺。 */
export type WatermarkPlacement =
  | 'tile'
  | 'topLeft'
  | 'topRight'
  | 'bottomLeft'
  | 'bottomRight'
  | 'bottomCenter'

/** 界面上给用户选的位置，顺序就是下拉里的顺序。 */
export const WATERMARK_PLACEMENTS: WatermarkPlacement[] = [
  'bottomRight',
  'bottomLeft',
  'bottomCenter',
  'topRight',
  'topLeft',
  'tile',
]

/** 单个印章离边缘的距离：按图片宽度的 2.5%，不会压到边上的文字。 */
export const WATERMARK_EDGE_RATIO = 0.025

export interface WatermarkOptions {
  /** 关掉时直接返回原图，不做任何处理。 */
  enabled: boolean
  text: string
  /** 5-100。越大越**不透明**（桌面程序的滑块标签写反了，这里按实际行为）。 */
  alpha: number
  /** 0 表示按图片宽度自动算。 */
  fontSize: number
  color: string
  /** 0 表示按 W/3 自动算（只在平铺模式下用）。 */
  stepX: number
  /** 0 表示按 H/4 自动算（只在平铺模式下用）。 */
  stepY: number
  /** 打在哪儿。默认右下角。 */
  placement: WatermarkPlacement
}

/**
 * 默认值。
 *
 * 【2026-10-22 主人要求】「水印不能默认为熊管家AI，可以没有」。
 * 原来照抄桌面程序写死 `熊管家AI`，但这里是辰数平台的网页，
 * 替用户署名到别人的图上是越权的。所以默认**留空**：
 * 留空时 `watermarkImageBlob` 直接返回原图，等于没水印；
 * 用户自己填了字才有水印。
 *
 * 【2026-10-24】位置默认右下角：单个、小、半透明，离边 2.5%，
 * 正常排版的主图右下角不会有正文，压不到字。
 */
export const DEFAULT_WATERMARK: WatermarkOptions = {
  enabled: false,
  text: '',
  alpha: 30,
  fontSize: 0,
  color: '#ffffff',
  stepX: 0,
  stepY: 0,
  placement: 'bottomRight',
}

/**
 * 判断「这次到底要不要真的打水印」。
 *
 * 开关开着但文字是空的 —— 没有字可画，等于没水印。
 * 单独抽出来是因为两个地方都要判（真正画图的地方 + 界面预览），
 * 分开写迟早会不一致。
 */
export function shouldApplyWatermark(options: WatermarkOptions): boolean {
  return options.enabled && options.text.trim().length > 0
}

/** 桌面程序的滑块范围。 */
export const WATERMARK_ALPHA_MIN = 5
export const WATERMARK_ALPHA_MAX = 100

/** 输出 JPEG 质量，和桌面程序的 92 对齐。 */
export const WATERMARK_JPEG_QUALITY = 0.92

/** 水印数量上限，防止用户把间距设得过小把浏览器卡死。 */
export const MAX_WATERMARK_TILES = 4000

/** 按图片宽度算字号：平铺模式沿用桌面程序的 max(14, W/30)。 */
export function resolveFontSize(width: number, configured: number): number {
  if (Number.isFinite(configured) && configured > 0) return configured
  return Math.max(14, Math.trunc(width / 30))
}

/**
 * 单个印章的字号。
 *
 * 比平铺模式小一档（W/45 而不是 W/30）：只有一个章，太大就喧宾夺主，
 * 而且更容易压到图里的字。主人要的是「不影响图片里文字的展示」。
 */
export function resolveStampFontSize(width: number, configured: number): number {
  if (Number.isFinite(configured) && configured > 0) return configured
  return Math.max(12, Math.trunc(width / 45))
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

export interface WatermarkStamp {
  x: number
  y: number
  align: CanvasTextAlign
  baseline: CanvasTextBaseline
}

/**
 * 算单个印章画在哪儿（纯数学，可单测）。
 *
 * 四个角都留 `WATERMARK_EDGE_RATIO` 的边距；底部居中的 x 是画面正中。
 * 用 textAlign/textBaseline 让「靠右」「靠下」由 canvas 自己算，
 * 这样不用去量文字宽度，不同字体下都不会跑偏。
 */
export function computeWatermarkStamp(
  width: number,
  height: number,
  placement: WatermarkPlacement,
): WatermarkStamp {
  const w = Number.isFinite(width) && width > 0 ? Math.floor(width) : 0
  const h = Number.isFinite(height) && height > 0 ? Math.floor(height) : 0
  if (!w || !h) return { x: 0, y: 0, align: 'left', baseline: 'top' }

  const margin = Math.max(1, Math.trunc(w * WATERMARK_EDGE_RATIO))

  switch (placement) {
    case 'topLeft':
      return { x: margin, y: margin, align: 'left', baseline: 'top' }
    case 'topRight':
      return { x: w - margin, y: margin, align: 'right', baseline: 'top' }
    case 'bottomLeft':
      return { x: margin, y: h - margin, align: 'left', baseline: 'bottom' }
    case 'bottomCenter':
      return { x: Math.trunc(w / 2), y: h - margin, align: 'center', baseline: 'bottom' }
    case 'bottomRight':
    default:
      return { x: w - margin, y: h - margin, align: 'right', baseline: 'bottom' }
  }
}

/** 把水印画到已经准备好尺寸的 canvas 2d 上下文上。 */
export function drawWatermark(
  ctx: CanvasRenderingContext2D,
  width: number,
  height: number,
  options: WatermarkOptions,
): WatermarkLayout {
  const text = options.text.trim()
  if (!text) return { tiles: [], truncated: false }

  // alpha 是 5-100，canvas 要 0-1。
  const alpha = Math.min(WATERMARK_ALPHA_MAX, Math.max(0, options.alpha)) / 100

  // 平铺模式：保留桌面程序的老行为（想满屏打的人还能用）。
  if (options.placement === 'tile') {
    const layout = computeWatermarkTiles(width, height, options)
    if (!layout.tiles.length) return layout

    const fontSize = resolveFontSize(width, options.fontSize)
    ctx.save()
    ctx.font = `${fontSize}px ${WATERMARK_FONT_STACK}`
    ctx.textAlign = 'left'
    ctx.textBaseline = 'top'
    ctx.globalAlpha = alpha
    for (const tile of layout.tiles) {
      // 桌面程序是纯白无描边，但那样压在浅色商品上完全看不见。
      // 这里加一圈淡描边保证任何底色都认得出——属于刻意的改进，已记录在案。
      ctx.lineWidth = Math.max(1, fontSize / 7)
      ctx.strokeStyle = 'rgba(0, 0, 0, 0.35)'
      ctx.strokeText(text, tile.x, tile.y)
      ctx.fillStyle = options.color
      ctx.fillText(text, tile.x, tile.y)
    }
    ctx.restore()
    return layout
  }

  // 单印章模式（默认）：一个，画在选定的位置。
  const fontSize = resolveStampFontSize(width, options.fontSize)
  const stamp = computeWatermarkStamp(width, height, options.placement)

  ctx.save()
  ctx.font = `${fontSize}px ${WATERMARK_FONT_STACK}`
  ctx.textAlign = stamp.align
  ctx.textBaseline = stamp.baseline
  ctx.globalAlpha = alpha
  ctx.lineWidth = Math.max(1, fontSize / 7)
  ctx.strokeStyle = 'rgba(0, 0, 0, 0.35)'
  ctx.strokeText(text, stamp.x, stamp.y)
  ctx.fillStyle = options.color
  ctx.fillText(text, stamp.x, stamp.y)
  ctx.restore()

  return { tiles: [{ x: stamp.x, y: stamp.y }], truncated: false }
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
