// Module-owned i18n for traceAccount (admin.traceAccount.*). Keep zh/en keys identical.
export default {
  traceAccount: {
    menuEntry: '追溯',
    title: '账号追溯：{name}',
    subtitle: '#{id} · {platform}',
    retry: '重试',
    loadFailed: '加载失败：{message}',
    none: '—',
    copy: '复制',
    copied: '已复制',
    tabs: {
      overview: '概览',
      status: '调度状态',
      ledger: '订阅台账'
    },
    range: {
      label: '时间窗',
      h1: '近 1 小时',
      h24: '近 24 小时',
      d7: '近 7 天'
    },
    overview: {
      failures: '失败次数',
      failuresHint: '窗口内上游失败尝试',
      distribution: '失败类型分布',
      lastFailure: '最近一次失败',
      noFailure: '窗口内无失败',
      fingerprints: '凭据指纹',
      fingerprintsHint: '窗口内用过的凭据版本数',
      attemptsTitle: '失败尝试',
      attemptsLimitHint: '最多显示最近 {limit} 条尝试',
      empty: '这个时间窗内没有失败尝试，账号运行正常。',
      fpSwitchedAt: '该指纹于 {time} 换入（见订阅台账）',
      requestIdMissing: '接口未返回 request_id',
      columns: {
        time: '时间',
        requestId: 'request_id',
        status: '状态码',
        kind: '失败类型',
        errorCode: '错误码',
        fingerprint: '凭据指纹',
        failover: '切换来源'
      },
      failoverFrom: '从 #{id} 切入',
      firstHop: '首跳'
    },
    kinds: {
      auth: '鉴权失败',
      rate_limit: '限流',
      overload: '过载',
      transient: '瞬时错误',
      client: '客户端错误'
    },
    status: {
      empty: '这个时间窗内没有调度状态变化（未被冷却或摘除）。',
      enterCooling: '进入冷却',
      enterInvalid: '被判失效（摘除）',
      enterState: '进入 {state}',
      coolingUntil: '冷却到 {time}',
      coolingDuration: '约 {minutes} 分钟',
      fromState: '原状态：{state}',
      reason: '原因：{reason}',
      errorCode: '错误码：{code}',
      triggeredBy: '触发请求',
      owner: '谁负责恢复：',
      sources: {
        classify: {
          label: '业务请求触发',
          desc: '一次真实请求的上游返回被判定为账号问题。',
          owner: '冷却会到期自动恢复；被判失效需管理员检查凭据或上游账号。'
        },
        refresh: {
          label: 'Token 刷新',
          desc: '后台刷新 OAuth token 失败。',
          owner: '管理员重新授权或更新凭据。'
        },
        manual: {
          label: '管理员手动',
          desc: '管理员在后台手动修改了调度状态。',
          owner: '由执行操作的管理员恢复。'
        },
        expiry: {
          label: '订阅到期',
          desc: '账号到期时间已过，被自动暂停。',
          owner: '续费并在账号设置里延长到期时间。'
        },
        scheduled_test: {
          label: '定时测试',
          desc: '定时连通性测试失败。',
          owner: '下次定时测试通过后自动恢复，持续失败需管理员排查。'
        },
        unknown: {
          label: '其它来源（{source}）',
          owner: '需人工判断。'
        }
      }
    },
    ledger: {
      hideTokenRefresh: '隐藏 token 刷新',
      hiddenCount: '已隐藏 {count} 条 token 刷新',
      total: '共 {count} 条事件',
      emptyTitle: '暂无台账记录',
      emptyDesc: '台账由数据库触发器写入。在台账启用之前创建、之后又没有改过受跟踪字段（状态、到期时间、套餐/额度、凭据等）的账号，台账是空的；下次修改这些字段时会开始记录。',
      allHidden: '当前只有 token 刷新事件，已被隐藏。',
      credentialChanged: '凭据指纹变化',
      credentialFailures: '此凭据在概览时间窗内失败 {count} 次',
      credentialNoFailures: '此凭据在概览时间窗内无失败',
      extended: '延长 {days} 天',
      shortened: '缩短 {days} 天',
      snapshot: '开通快照',
      statusAfter: '变更后状态：{status}',
      source: '来源：{source}',
      operator: '操作人 #{id}',
      emptyValue: '（空）',
      yes: '是',
      no: '否',
      events: {
        created: '开通',
        deleted: '删除',
        restored: '恢复（撤销删除）',
        expired: '到期暂停',
        renewed: '续费',
        expiry_shortened: '缩短到期',
        expiry_set: '设置到期时间',
        expiry_cleared: '取消到期时间',
        plan_changed: '改套餐',
        quota_changed: '改额度',
        disabled: '禁用',
        errored: '进入错误状态',
        recovered: '从错误恢复',
        enabled: '启用',
        credential_rotated: '换 key',
        token_refreshed: 'token 刷新',
        unschedulable: '停止调度',
        schedulable: '恢复调度',
        updated: '其它修改'
      },
      fields: {
        status: '状态',
        schedulable: '可调度',
        expires_at: '到期时间',
        auto_pause_on_expired: '到期自动暂停',
        deleted_at: '删除时间',
        proxy_id: '代理',
        concurrency: '并发',
        priority: '优先级',
        rate_multiplier: '倍率',
        name: '名称',
        credential_fp: '凭据指纹',
        platform: '平台',
        type: '类型',
        extraPrefix: '套餐/额度 · {key}'
      }
    }
  }
}
