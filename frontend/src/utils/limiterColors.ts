/**
 * limiterColors.ts — 服务限额 limiter 类型颜色规范的单点定义。
 *
 * 三个页面（限额监控 / 限额配置 / 用户限额查看）都用同一套配色，避免
 * "管理员看到 RPM=紫色，配置页看到 RPM=蓝色"这种漂移导致的认知割裂。
 *
 * 配色（与传统业务习惯对齐 + 用户明确要求 concurrency=黄色）：
 *   - rpm         蓝色   每分钟请求频率
 *   - tpm         紫色   每分钟 token 数（与 RPM 区分开）
 *   - tpd         靛蓝   每日 token 数
 *   - daily_usd   绿色   美元金额（绿 = 钱）
 *   - concurrency 黄色   并发数（用户要求）
 *   - 其他        灰色
 */

/**
 * 浅色背景 + 深色文字的 chip class（监控页 + 配置页通用）。
 * 故意只暴露 inline tailwind class，避免引入"badge-blue"这类预设依赖；
 * 让限额功能的 chip 视觉自治，不被全局 .badge-* 改样式波及。
 */
export function limiterChipClass(type: string): string {
  switch (type) {
    case 'rpm':
      return 'bg-blue-100 text-blue-700 dark:bg-blue-500/20 dark:text-blue-300'
    case 'tpm':
      return 'bg-violet-100 text-violet-700 dark:bg-violet-500/20 dark:text-violet-300'
    case 'tpd':
      return 'bg-indigo-100 text-indigo-700 dark:bg-indigo-500/20 dark:text-indigo-300'
    case 'daily_usd':
      return 'bg-emerald-100 text-emerald-700 dark:bg-emerald-500/20 dark:text-emerald-300'
    case 'concurrency':
      return 'bg-yellow-100 text-yellow-700 dark:bg-yellow-500/20 dark:text-yellow-300'
    default:
      return 'bg-gray-100 text-gray-700 dark:bg-gray-500/20 dark:text-gray-300'
  }
}
