// Module-owned i18n for traceRenewal (admin.traceRenewal.*). Keep zh/en keys identical.
export default {
  traceRenewal: {
    title: '待续费账号',
    refresh: '刷新',
    windowLabel: '窗口',
    windowDays: '{n} 天',
    loading: '加载中…',
    loadFailed: '加载待续费账号失败',
    retry: '重试',
    empty: '{days} 天内没有需要续费的账号',
    alertScopeHint:
      '告警指标「账号即将到期数」只统计 72 小时内到期 + 已过期仍在调度的账号，当前计入 {n} 个；已暂停的过期账号不计入。',
    groups: {
      expiredActive: '已过期仍在调度',
      expiredPaused: '已过期已暂停',
      within24h: '24 小时内',
      within3d: '3 天内',
      within7d: '7 天内',
      later: '7 天以上'
    },
    groupHints: {
      expiredActive: '仍会接流量，但凭据/订阅已失效，请立即续费或暂停',
      expiredPaused: '已不再接流量，续费后需手动恢复调度'
    },
    columns: {
      name: '账号',
      platform: '平台',
      remaining: '剩余时间',
      expiresAt: '到期时间',
      autoPause: '到期自动暂停',
      scheduling: '调度状态'
    },
    remaining: {
      leftMinutes: '还剩 {n} 分钟',
      leftHours: '还剩 {n} 小时',
      leftDays: '还剩 {n} 天',
      expiredMinutes: '已过期 {n} 分钟',
      expiredHours: '已过期 {n} 小时',
      expiredDays: '已过期 {n} 天'
    },
    autoPauseOn: '已开启',
    autoPauseOff: '未开启',
    scheduling: {
      active: '调度中',
      paused: '已暂停',
      disabled: '已禁用',
      error: '异常'
    },
    openAccount: '在账号管理中查看'
  }
}
