export default {
  groupsStatus: {
    title: '分组状态',
    description: '查看所有公开分组的实时账号可用情况与计费倍率',
    overview: '共 {groups} 个公开分组，{available} 个当前可用',
    lastUpdated: '更新于 {time}',
    refresh: '刷新状态',
    retry: '重新加载',
    loading: '正在加载分组状态',
    loadFailed: '分组状态加载失败，请稍后重试。',
    empty: '当前没有公开分组',
    noResults: '没有符合当前筛选条件的分组',
    publicOnly: '这里只展示对所有用户公开的分组，不会显示专属授权分组。',
    filters: {
      title: '筛选分组',
      resultSummary: '当前显示 {shown} / {total} 个分组',
      searchLabel: '搜索',
      searchPlaceholder: '搜索分组名称或描述',
      clearSearch: '清空搜索',
      channelLabel: '渠道',
      statusLabel: '状态',
      allChannels: '全部渠道',
      allStatuses: '全部状态',
      reset: '重置筛选'
    },
    summary: {
      accounts: '账号总数',
      available: '可用账号',
      rateLimited: '限流账号',
      availabilityRate: '总体可用率'
    },
    table: {
      group: '分组',
      channel: '渠道',
      rate: '倍率',
      accounts: '账号总数',
      available: '可用数量',
      rateLimited: '限流数量',
      status: '可用状态'
    },
    status: {
      available: '可用',
      degraded: '部分限流',
      rate_limited: '限流中',
      unavailable: '不可用'
    },
    statusHint: {
      available: '分组存在可调度账号',
      degraded: '分组可用，但部分账号正在限流',
      rate_limited: '当前没有可用账号，账号处于临时限流状态',
      unavailable: '分组已停用或当前没有可调度账号'
    },
    accountBreakdown: '{available} 可用 · {limited} 限流 · {other} 其他不可用'
  }
}
