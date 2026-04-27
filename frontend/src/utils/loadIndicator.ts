/**
 * 共享的负载指标可视化工具
 * 用于把负载百分比映射到 Tailwind 颜色 class 和进度条 inline style
 *
 * 阈值（与 OpsConcurrencyCard 历史保持一致）：
 *   - >= 90%  → 红
 *   - >= 70%  → 橙
 *   - >= 50%  → 黄
 *   - 其他    → 绿
 *
 * NaN/负数/超 100 等异常值由各函数内部夹紧或按"无负载"处理。
 */

const RED_THRESHOLD = 90
const ORANGE_THRESHOLD = 70
const YELLOW_THRESHOLD = 50

function clampPct(loadPct: number): number {
  if (!Number.isFinite(loadPct)) return 0
  if (loadPct <= 0) return 0
  if (loadPct >= 100) return 100
  return loadPct
}

/**
 * 进度条填充色 class（背景色）
 * NaN / 负数会落到绿色区段
 */
export function getLoadBarClass(loadPct: number): string {
  if (Number.isFinite(loadPct) && loadPct >= RED_THRESHOLD) return 'bg-red-500 dark:bg-red-600'
  if (Number.isFinite(loadPct) && loadPct >= ORANGE_THRESHOLD) return 'bg-orange-500 dark:bg-orange-600'
  if (Number.isFinite(loadPct) && loadPct >= YELLOW_THRESHOLD) return 'bg-yellow-500 dark:bg-yellow-600'
  return 'bg-green-500 dark:bg-green-600'
}

/**
 * 进度条宽度的 inline style 字符串（百分比值已夹紧到 [0, 100]）
 */
export function getLoadBarStyle(loadPct: number): string {
  return `width: ${clampPct(loadPct)}%`
}

/**
 * 进度条填充色 class（半透明版）
 *
 * 用于"文字与进度条重叠"的样式：填充条用 30%(亮模式) / 40%(暗模式) 透明度，
 * 让覆盖在上层的数字/百分比文字始终可读。与 getLoadBarClass 同色阶但弱化。
 */
export function getLoadBarOverlayClass(loadPct: number): string {
  if (Number.isFinite(loadPct) && loadPct >= RED_THRESHOLD) return 'bg-red-500/30 dark:bg-red-500/40'
  if (Number.isFinite(loadPct) && loadPct >= ORANGE_THRESHOLD) return 'bg-orange-500/30 dark:bg-orange-500/40'
  if (Number.isFinite(loadPct) && loadPct >= YELLOW_THRESHOLD) return 'bg-yellow-500/30 dark:bg-yellow-500/40'
  return 'bg-green-500/30 dark:bg-green-500/40'
}

/**
 * 数值文字的颜色 class
 */
export function getLoadTextClass(loadPct: number): string {
  if (Number.isFinite(loadPct) && loadPct >= RED_THRESHOLD) return 'text-red-600 dark:text-red-400'
  if (Number.isFinite(loadPct) && loadPct >= ORANGE_THRESHOLD) return 'text-orange-600 dark:text-orange-400'
  if (Number.isFinite(loadPct) && loadPct >= YELLOW_THRESHOLD) return 'text-yellow-600 dark:text-yellow-400'
  return 'text-green-600 dark:text-green-400'
}
