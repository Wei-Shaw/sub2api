/**
 * Chinese i18n — `channelStatus.*` namespace (V5 W7.3 user-facing read-only view).
 *
 * T19 Fix B — split out from monolithic `zh.ts`.
 */
export default {
  title: '渠道状态',
  description: '查看渠道可用性、延迟和近期状态',
  searchPlaceholder: '搜索渠道...',
  allProviders: '全部平台',
  loadError: '加载渠道状态失败',
  detailLoadError: '加载渠道详情失败',
  detailTitle: '渠道详情',
  closeDetail: '关闭',
  windowTab: {
    '7d': '7 天',
    '15d': '15 天',
    '30d': '30 天',
  },
  overall: {
    operational: 'OPERATIONAL',
    degraded: 'DEGRADED',
    unavailable: 'UNAVAILABLE',
  },
  columns: {
    name: '名称',
    provider: '平台',
    groupName: '分组',
    primaryModel: '主模型',
    availability7d: '7 天可用率',
    latency: '延迟 (ms)',
  },
  detailColumns: {
    model: '模型',
    latestStatus: '最新状态',
    latestLatency: '最新延迟 (ms)',
    availability7d: '7 天可用率',
    availability15d: '15 天可用率',
    availability30d: '30 天可用率',
    avgLatency7d: '7 天平均延迟 (ms)',
  },
  empty: {
    title: '暂无渠道',
    description: '管理员尚未配置任何监控渠道.',
  },
}
