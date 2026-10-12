export default {
  videoTasks: {
    title: '视频任务',
    description: '只读查看任务进度、失败原因与计费状态',
    coverage: '自动跟进已记录视频任务的生成状态。提交结果未知的记录需向上游核对，不会自动重新生成。',
    billingNote: '扣费情况与使用记录同步。本页仅用于进度监控和分析，不会发起扣费或补扣；预估费用不代表已扣费。',
    elapsed: '{hours} 小时 {minutes} 分 {seconds} 秒',
    search: '搜索',
    searchPlaceholder: '任务 / 模型 / 用户 / 账号（按原文匹配）',
    status: '状态',
    allStatuses: '全部状态',
    recent: '时间范围',
    days: '最近 {count} 天',
    empty: '没有符合筛选条件的视频任务。',
    loadFailed: '视频任务加载失败，请刷新重试。',
    detail: '任务详情',
    columns: {
      task: '任务 / 模型', user: '用户 / API 密钥', account: '账号 / 分组',
      status: '状态 / 说明', billing: '计费', created: '创建时间', actions: '详情'
    },
    fields: {
      taskId: '任务 ID', kind: '记录类型', model: '模型', user: '用户', userId: '用户 ID',
      apiKey: 'API 密钥名称', apiKeyId: 'API 密钥 ID', account: '账号', accountId: '账号 ID',
      group: '分组', groupId: '分组 ID', expected: '预估费用', charged: '已扣费用',
      created: '创建时间', updated: '更新时间', completed: '完成时间', nextCheck: '下次检查时间',
      attempts: '处理尝试次数', duration: '时长（秒）', resolution: '分辨率', usageLogId: '用量记录 ID', elapsed: '耗时（截至完成或最近更新）'
    },
    kinds: { task: '任务', submission: '结果未确认的提交' },
    statuses: {
      in_progress: '进行中', delayed: '已延迟', query_retry: '查询待重试',
      billing_pending: '计费待处理', completed: '已完成', failed: '失败',
      submission_unknown: '提交结果未知', review_required: '待核对', unknown: '状态未知'
    },
    explanations: {
      in_progress: '任务仍在处理中。',
      delayed: '待处理时间已超过两小时，不代表任务已失败。',
      query_retry: '状态查询未完成，系统将再次查询，不会因此重新生成视频。',
      billing_pending: '正在核对完成结果或处理计费，已扣费情况请以计费状态为准。',
      completed: '任务处理已完成。',
      failed: '上游任务未成功完成；若未扣费，则无需退款。',
      submission_unknown: '可能仍在提交，也可能提交结果尚未确认。请勿更换幂等键再次提交，否则可能重复生成视频。站内计费待核查，上游可能已经产生费用。',
      review_required: '自动查询已暂停，需向上游核对结果；不代表生成失败，不会自动重新生成或补扣。',
      unknown: '暂时没有可识别的任务状态。'
    },
    billing: { not_charged: '未扣费', pending: '待结算', charged: '已扣费', review: '待核查', unknown: '计费状态未知' },
    reasons: {
      poll_failed: '最近一次状态查询失败，系统将再次检查。',
      poll_timeout: '状态查询超时，不代表任务失败，系统将再次检查。',
      poll_not_found: '上游暂时无法找到该任务，系统将有限重试，不会重新生成。',
      poll_exhausted: '任务超过一天且已多次查询，仍无法确认结果，已停止自动重试。',
      account_route_changed: '任务原上游账号或线路已变更，已暂停查询，避免关联到错误的结果。',
      upstream_failed: '上游服务报告任务失败。',
      upstream_expired: '上游报告结果已过期，生成及费用结果仍待核查，不会重新生成视频。',
      upstream_cancelled: '上游服务报告任务已取消。',
      completion_save_failed: '完成记录保存失败，系统将再次核对。',
      billing_failed: '计费结算暂未确认，系统将再次核对，不会重新生成视频。',
      unknown: '记录了其他错误，暂时没有可安全展示的详细说明。'
    }
  }
}
