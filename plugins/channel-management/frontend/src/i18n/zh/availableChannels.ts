/**
 * Chinese i18n — `availableChannels.*` namespace (V5 W9 user-facing list).
 *
 * T19 Fix B — split out from monolithic `zh.ts`.
 */
export default {
  title: '可用渠道',
  description: '您可以访问的渠道及其支持的模型与定价',
  searchPlaceholder: '搜索渠道或模型...',
  empty: '暂无可用渠道',
  noModels: '未配置模型',
  noPricing: '未配置定价',
  exclusive: '专属',
  public: '公开',
  exclusiveTooltip: '管理员授予的专属分组',
  publicTooltip: '对所有用户开放的分组',
  columns: {
    name: '渠道',
    description: '描述',
    platform: '平台',
    groups: '可访问分组',
    supportedModels: '支持模型',
  },
  pricing: {
    billingMode: '计费方式',
    billingModeToken: '按 Token',
    billingModePerRequest: '按请求',
    billingModeImage: '按图片',
    inputPrice: '输入',
    outputPrice: '输出',
    cacheWritePrice: '缓存写入',
    cacheReadPrice: '缓存读取',
    imageOutputPrice: '图片输出',
    perRequestPrice: '每次请求',
    intervals: '阶梯定价',
    unitPerMillion: '/ 百万 tokens',
    unitPerRequest: '/ 次请求',
  },
}
