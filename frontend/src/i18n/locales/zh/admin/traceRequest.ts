// Module-owned i18n for traceRequest (admin.traceRequest.*). Keep zh/en keys identical.
export default {
  traceRequest: {
    tabDetail: '详情',
    tabTrace: '追溯',
    tabTraceDisabled: '这条错误没有 request_id，无法追溯',
    title: '请求链路',
    refresh: '刷新',
    loading: '正在加载追溯记录…',
    noRequestId: '这条错误没有 request_id，无法查询请求链路。',
    empty: '这条请求没有留下追溯记录',
    emptyHint: '追溯能力上线之前产生的错误、或未经过上游转发的请求不会有记录。',
    loadFailed: '加载追溯记录失败',
    retry: '重试',
    noAttempts: '这条请求没有留下逐跳记录：可能是追溯能力上线之前的老错误，或请求未转发到上游。',
    noAttemptsSuccess: '请求一次成功，未记录上游尝试（只有最终失败的请求才会记录每一跳）。',
    attemptNo: '第 {n} 跳',
    account: '账号 #{id}',
    accountUnknown: '账号未知',
    copyAccountId: '点击复制账号 ID',
    copiedAccountId: '已复制账号 ID {id}',
    switchFromTo: '换号：从账号 #{from} 切换到账号 #{to}',
    credentialFpHint: '凭据指纹：同一账号指纹变化说明凭据已轮换',
    expand: '展开',
    collapse: '收起',
    otherStatusEvents: '其它账号状态变化',
    summary: {
      attempts: '共 {n} 跳',
      switches: '换号 {n} 次',
      succeeded: '最终成功',
      failed: '最终失败'
    },
    fields: {
      errorCode: '错误码',
      elapsed: '耗时（距上一跳）',
      model: '模型',
      credentialFp: '凭据指纹',
      proxy: '代理',
      accountState: '账号状态',
      upstreamUrl: '上游地址'
    },
    kind: {
      none: '正常',
      auth: '鉴权失败',
      rate_limit: '限流',
      overload: '过载',
      transient: '临时错误',
      client: '客户端错误'
    },
    statusEvent: {
      coolingUntil: '账号 #{id} 进入冷却，直到 {until}',
      invalid: '账号 #{id} 被判定为失效（摘除调度）',
      changed: '账号 #{id} 状态：{from} → {to}'
    },
    source: {
      classify: '错误分类',
      refresh: '令牌刷新',
      manual: '手动操作',
      expiry: '到期',
      scheduled_test: '定时测试'
    },
    final: {
      success: '最终成功',
      failure: '最终失败',
      account: '账号',
      model: '模型',
      tokens: '输入 / 输出 tokens',
      duration: '总耗时',
      firstToken: '首字耗时',
      cost: '费用',
      upstreamStatus: '上游 {code}',
      phase: '阶段',
      type: '类型'
    }
  }
}
