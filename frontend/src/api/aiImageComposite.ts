/**
 * 一图多视（桌面程序的「拼接」功能）。
 *
 * 用途：一次上传商品的多张照片（正面/侧面/背面），横向拼成一张长图，
 * 作为**第 1 张**图的参考图送进生图接口，让模型一次看全商品。
 * 后续第 2~N 张改用第 1 张的生成结果当参考，保证整组图商品一致。
 *
 * 参数全部照抄桌面程序（已反汇编确认）：
 *   - **横排一行**，不是网格
 *   - 统一缩放到**高 800px**，宽度按原比例算
 *   - 图与图之间 **20px**（不是每张右边 20px）
 *   - 白底、输出 PNG
 *   - 1 张时直接返回原图，不做任何处理
 *
 * 排布计算是**纯函数**，可以单测；真正画图才碰 canvas。
 */

export interface CompositeOptions {
  /** 统一缩放到的高度。桌面程序写死 800。 */
  height: number
  /** 图与图之间的水平间距。桌面程序写死 20。 */
  gap: number
  /** 背景色。 */
  background: string
}

export const DEFAULT_COMPOSITE: CompositeOptions = {
  height: 800,
  gap: 20,
  background: '#ffffff',
}

/**
 * 张数上限。
 *
 * 桌面程序**没有**限制，但横排 N 张 3:4 竖图拼出来宽度是 N*600+20*(N-1)，
 * 10 张就是 6180px —— 生图接口普遍有边长上限，所以这里加一道闸门。
 * 超出时取前 N 张，并在界面提示用户。
 */
export const MAX_COMPOSITE_IMAGES = 6

export interface CompositeCell {
  x: number
  y: number
  width: number
  height: number
}

export interface CompositeLayout {
  /** 画布总宽。 */
  width: number
  height: number
  cells: CompositeCell[]
}

/**
 * 计算横排排布。
 *
 * `sizes` 是每张图的原始宽高（按用户选择的顺序）。
 * 每张等比缩放到高 height，宽 = 原宽 * height / 原高，然后依次横排、间隔 gap。
 */
export function computeCompositeLayout(
  sizes: Array<{ width: number; height: number }>,
  options: CompositeOptions = DEFAULT_COMPOSITE,
): CompositeLayout {
  const usable = sizes
    .slice(0, MAX_COMPOSITE_IMAGES)
    .filter((size) => size.width > 0 && size.height > 0)

  if (!usable.length) {
    return { width: 0, height: 0, cells: [] }
  }

  const targetHeight = options.height > 0 ? options.height : DEFAULT_COMPOSITE.height
  const gap = options.gap >= 0 ? options.gap : DEFAULT_COMPOSITE.gap

  const cells: CompositeCell[] = []
  let x = 0
  for (const size of usable) {
    // 等比缩放到统一高度，宽度按比例算（桌面程序用 int() 截断，这里用 round 更准）。
    const width = Math.round((size.width * targetHeight) / size.height)
    cells.push({ x, y: 0, width, height: targetHeight })
    x += width + gap
  }

  // 最后一个右边没有 gap，所以总宽要减掉一个 gap。
  const totalWidth = x - gap

  return { width: totalWidth, height: targetHeight, cells }
}

/** 把多张图片拼成一张长图，返回 PNG Blob。 */
export async function compositeImageBlobs(
  sources: Blob[],
  options: CompositeOptions = DEFAULT_COMPOSITE,
): Promise<Blob> {
  const usable = sources.slice(0, MAX_COMPOSITE_IMAGES)
  if (!usable.length) throw new Error('至少需要一张图片才能拼接')

  // 只有一张就不拼了，直接返回原图（和桌面程序一致，避免无谓的重编码）。
  if (usable.length === 1) return usable[0]

  const bitmaps = await Promise.all(usable.map((blob) => createImageBitmap(blob)))
  try {
    const layout = computeCompositeLayout(
      bitmaps.map((bitmap) => ({ width: bitmap.width, height: bitmap.height })),
      options,
    )
    if (!layout.width || !layout.height) throw new Error('图片尺寸不合法，无法拼接')

    const canvas = document.createElement('canvas')
    canvas.width = layout.width
    canvas.height = layout.height
    const ctx = canvas.getContext('2d')
    if (!ctx) throw new Error('浏览器不支持 canvas 2d，无法拼接图片')

    // LANCZOS 在浏览器里对应 imageSmoothingQuality = 'high'，缩放质量更接近原程序。
    ctx.imageSmoothingEnabled = true
    ctx.imageSmoothingQuality = 'high'
    ctx.fillStyle = options.background
    ctx.fillRect(0, 0, canvas.width, canvas.height)

    bitmaps.forEach((bitmap, index) => {
      const cell = layout.cells[index]
      if (!cell) return
      ctx.drawImage(bitmap, cell.x, cell.y, cell.width, cell.height)
    })

    const output = await new Promise<Blob | null>((resolve) => canvas.toBlob(resolve, 'image/png'))
    if (!output) throw new Error('导出拼接图失败')
    return output
  } finally {
    bitmaps.forEach((bitmap) => bitmap.close?.())
  }
}
