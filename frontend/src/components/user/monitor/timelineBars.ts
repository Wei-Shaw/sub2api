import type { MonitorTimelinePoint, MonitorStatus } from '@/api/channelMonitor'

export interface TimelineBar {
  colorClass: string
  heightPct: number
  title: string
}

// 4 级高度 + 颜色双重编码：高=好+绿，短=坏+红，灰=未测试。
// 长绿(正常) > 中黄(降级) > 短红(失败/系统错误) > 很短灰(未测试)。
const STATUS_HEIGHT: Record<string, number> = {
  operational: 100,
  degraded: 65,
  failed: 35,
  error: 35,
  empty: 15,
}

const STATUS_COLOR: Record<string, string> = {
  operational: 'bg-emerald-500',
  degraded: 'bg-amber-500',
  failed: 'bg-red-500',
  error: 'bg-red-500',
  empty: 'bg-gray-300 dark:bg-dark-600',
}

export interface TimelineFormatters {
  formatLatency: (ms?: number | null) => string
  formatRelativeTime: (iso?: string | null) => string
  statusLabel: (status: MonitorStatus | '') => string
}

/**
 * 把服务端返回的「最新在前」时间线点转换成「最旧在左、现在在右」的柱子序列。
 * 不足 length 时在左侧补未测试占位柱子，保证柱子数量恒定。
 */
export function buildTimelineBars(
  buckets: MonitorTimelinePoint[] | undefined,
  length: number,
  fmt: TimelineFormatters
): TimelineBar[] {
  const real = [...(buckets ?? [])].slice(0, length).reverse()
  const padCount = Math.max(0, length - real.length)
  const bars: TimelineBar[] = []

  for (let i = 0; i < padCount; i += 1) {
    bars.push({
      colorClass: STATUS_COLOR.empty,
      heightPct: STATUS_HEIGHT.empty,
      title: '',
    })
  }

  for (const point of real) {
    const status = point.status as keyof typeof STATUS_HEIGHT
    const colorClass = STATUS_COLOR[status] ?? STATUS_COLOR.empty
    const heightPct = STATUS_HEIGHT[status] ?? STATUS_HEIGHT.empty
    const latency = fmt.formatLatency(point.latency_ms)
    const relative = fmt.formatRelativeTime(point.checked_at)
    const label = fmt.statusLabel(point.status)
    bars.push({
      colorClass,
      heightPct,
      title: `${relative} · ${label} · ${latency}ms`,
    })
  }

  return bars
}
