export default {
  cpa: {
    title: 'CPA 面板',
    description: '本机 CLIProxyAPI 的只读视图（凭据、额度、用量、日志）',
    notConfigured: 'CPA 面板尚未配置，请在下方填写 base URL 与 management key。',
    disabled: 'CPA 面板已关闭，开启后才会加载数据。',

    connection: {
      title: '连接配置',
      baseUrl: 'CPA Base URL',
      baseUrlHint: '从 Sub2API 容器内可达，例如 http://host.docker.internal:8317',
      managementKey: 'Management Key',
      managementKeyPlaceholder: '输入即更新，留空表示保留当前 key',
      managementKeyKeep: '已保存 key（{hint}），留空表示不修改。',
      managementKeyClear: '清空已保存的 key',
      enabled: '启用 CPA 面板',
      save: '保存',
      saved: 'CPA 配置已保存',
      test: '测试连接',
      testing: '测试中…',
      reachable: '可达',
      unreachable: '不可达',
      healthOk: '健康检查通过',
      keyAccepted: 'Management key 有效',
      keyRejected: 'Management key 无效',
      version: '版本',
      commit: 'Commit',
      buildDate: '构建时间',
      supportPlugin: '插件支持',
      neverTested: '尚未测试'
    },

    tabs: {
      authFiles: '凭据与额度',
      usage: '使用情况',
      logs: '日志'
    },

    authFiles: {
      empty: 'CPA 没有返回任何 auth file。',
      name: '凭据文件',
      provider: '类型',
      status: '状态',
      account: '账号',
      success: '成功',
      failed: '失败',
      lastRefresh: '最近刷新',
      nextRetry: '下次重试',
      priority: '优先级',
      note: '备注',
      plan: '套餐',
      refreshQuota: '查询额度',
      refreshAllQuota: '查询全部额度',
      quotaLoading: '正在查询额度…',
      quotaError: '额度查询失败：{message}',
      quotaEmpty: '未返回额度数据',
      fiveHour: '5 小时窗口',
      weekly: '每周窗口',
      monthly: '每月窗口',
      codeReview: '代码审查',
      additional: '附加限额：{name}',
      resetCredits: '重置次数',
      passiveSignals: '被动额度信号',
      noSignals: '尚未观测到被动信号',
      observedAt: '观测时间 {time}',
      recentRequests: '最近请求',
      unavailable: '不可用',
      disabled: '已禁用'
    },

    usage: {
      empty: 'CPA 没有返回上游 API key 的用量。',
      hint: 'CPA 只对 API-key 类上游统计用量；OAuth 凭据请看「凭据与额度」页。',
      provider: '提供商',
      key: '上游 Key',
      success: '成功',
      failed: '失败',
      recent: '最近分桶'
    },

    logs: {
      requestLogs: '请求日志',
      requestLogsDisabled: 'CPA 未开启 logging-to-file，请求日志不可用；如需查看请在 CPA 侧开启。',
      errorLogs: '错误响应日志',
      empty: '暂无错误日志。',
      fileName: '文件',
      size: '大小',
      modified: '修改时间',
      view: '查看',
      contentTitle: '日志内容：{name}',
      contentEmpty: '（空）',
      close: '关闭'
    },

    common: {
      refresh: '刷新',
      loading: '加载中…',
      error: '加载失败：{message}',
      enabled: '已启用',
      disabled: '已禁用',
      yes: '是',
      no: '否',
      unknown: '未知'
    }
  }
}
