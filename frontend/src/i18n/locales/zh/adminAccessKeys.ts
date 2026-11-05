export default {
  "title": "管理员访问密钥",
  "authorizationHint": "按操作和资源范围授权；未勾选操作不生效。",
  "sharedAccountWarning": "共享账户操作会影响所有关联分组；全站权限不受分组限制。",
  "secretWarning": "完整密钥仅展示本次，请立即复制保存。关闭后无法再次查看。",
  "copySecret": "复制密钥",
  "dismissSecret": "已保存，关闭展示",
  "create": "创建管理密钥",
  "refresh": "刷新列表",
  "loading": "正在加载密钥及权限目录…",
  "noDescription": "无备注",
  "never": "永不过期",
  "neverUsed": "尚未使用",
  "summaryLabel": "授权摘要",
  "editPermissions": "编辑权限",
  "disable": "停用",
  "enable": "启用",
  "rotate": "轮换",
  "revoke": "永久撤销",
  "deleteRecord": "删除记录",
  "editTitle": "编辑密钥",
  "createTitle": "创建密钥",
  "name": "名称",
  "description": "备注",
  "expiration": "有效期（本地时间，留空永不过期）",
  "allowedIps": "允许的 IP / CIDR（每行一个，留空不限制）",
  "enabled": "启用密钥",
  "allGroups": "所有分组（包括未来分组）",
  "groups": "分组（可多选）",
  "includeUngrouped": "包含未分组账户",
  "accountTypes": "账户类型（仅限制账户操作，未选表示无账户权限）",
  "platforms": "平台（可多选，未选表示不限平台）",
  "accountOperations": "允许的账户操作",
  "groupOperations": "允许的分组操作（受上方分组与平台限制）",
  "modelAllowlistHint": "分组模型白名单会限制分组可调用的模型。",
  "removeRule": "删除规则",
  "addRule": "添加资源规则",
  "globalPermissions": "全站权限（默认全部关闭）",
  "processing": "处理中…",
  "save": "保存密钥",
  "cancel": "取消",
  "other": "其他",
  "allGroupsSummary": "所有分组（含未来分组）",
  "noGroups": "无分组",
  "ungroupedSummary": "，含未分组",
  "noAccountTypes": "未选择类型（无账户权限）",
  "anyPlatform": "不限平台",
  "operationFailed": "操作失败，请重试",
  "nameRequired": "请输入密钥名称",
  "expirationInvalid": "请输入有效的到期时间",
  "saved": "密钥已保存",
  "statusUpdated": "密钥状态已更新",
  "rotated": "密钥已轮换",
  "revoked": "密钥已永久撤销",
  "removed": "密钥已从列表移除，历史操作记录仍会继续保留",
  "copied": "密钥已复制",
  "copyFailed": "复制失败，请手动复制密钥",
  "statusRevoked": "已撤销",
  "statusDisabled": "已停用",
  "statusExpired": "已过期",
  "statusEnabled": "已启用",
  "summary": "全站权限：{globals} 项 · 资源规则：{rules} 条",
  "ruleLine": "规则 {index}：{summary}",
  "expires": "到期：{value}",
  "lastUsed": "最近使用：{value}",
  "ruleLegend": "账户与分组资源规则 {index}",
  "groupFallback": "分组 #{id}",
  "ruleSummary": "{groups}；账户类型：{types}（仅限制账户操作）；平台：{platforms}；操作：{count} 项",
  "rotateConfirm": "轮换“{name}”将立即使原密钥失效，是否继续？",
  "revokeConfirm": "永久撤销“{name}”后无法恢复，是否继续？",
  "deleteConfirm": "确定删除“{name}”的密钥记录？删除只是从列表移除，历史操作记录仍会继续保留。",
  "legacy": "旧版全权密钥兼容区（建议迁移到受限密钥）",
  "fullAccessLabel": "全权访问（兼容旧版管理员 Key）",
  "fullAccessWarning": "开启后，此密钥可访问全部管理资源、读取敏感数据并修改账户和财务配置；未来新增的管理接口也将允许访问。分组、平台、账户类型和操作规则不再限制此密钥。仅限管理员登录会话及二次验证的接口仍遵循原要求。",
  "rulesSuspended": "资源规则已暂停，关闭全权访问后恢复；有效期、IP 限制、限流和审计继续生效。",
  "fullAccessConfirmTitle": "确认开启全权访问",
  "fullAccessPhrase": "开启全权访问",
  "fullAccessPhraseHint": "请输入“{phrase}”以确认：",
  "unknownPermission": "未知权限（{id}）",
  "permissions": {
    "accounts_didentity_dread": {
      "label": "读取账户邮箱、套餐及创建时间（不含令牌）",
      "category": "账户"
    },
    "accounts_dusage_dread": {
      "label": "读取及主动刷新授权账户额度与快照",
      "category": "账户"
    },
    "accounts_dstats_dread": {
      "label": "读取账户用量与费用统计",
      "category": "账户"
    },
    "openai_dquota_dread": {
      "label": "查询 OpenAI 上游额度与重置次数",
      "category": "OpenAI"
    },
    "openai_dquota_dreset": {
      "label": "消耗 OpenAI 重置次数恢复额度",
      "category": "OpenAI"
    },
    "openai_dreferrals_dread": {
      "label": "查询 OpenAI 邀请资格与次数",
      "category": "OpenAI"
    },
    "openai_dreferrals_dinvite": {
      "label": "发送 OpenAI 邀请邮件",
      "category": "OpenAI"
    },
    "accounts_doptions_dwrite": {
      "label": "写入 OpenAI OAuth 身份与连接选项",
      "category": "账户"
    },
    "accounts_drate_dwrite": {
      "label": "设置账户计费倍率",
      "category": "账户"
    },
    "groups_dlist_dread": {
      "label": "读取所选分组基本信息（名称、平台、状态）",
      "category": "分组"
    },
    "groups_dcapacity_dread": {
      "label": "读取所选分组容量（整个分组，不按账户类型过滤）",
      "category": "分组"
    },
    "openai_dtokens_drefresh": {
      "label": "刷新调用方提供的 OpenAI 令牌（返回令牌，可使用所选代理）",
      "category": "OAuth"
    },
    "accounts_dread": {
      "label": "查看账户（脱敏）",
      "category": "账户"
    },
    "accounts_dcreate": {
      "label": "创建账户",
      "category": "账户"
    },
    "accounts_dupdate": {
      "label": "编辑名称与备注",
      "category": "账户"
    },
    "accounts_dcredentials": {
      "label": "写入凭证（不包含读取）",
      "category": "账户"
    },
    "accounts_dmodels_dwrite": {
      "label": "编辑账户白名单模型 ID 与映射",
      "category": "账户"
    },
    "accounts_dnetwork": {
      "label": "修改接入地址、代理与连接选项",
      "category": "账户"
    },
    "accounts_dstatus": {
      "label": "启用、停用与调度开关",
      "category": "账户"
    },
    "accounts_dschedule": {
      "label": "修改并发、优先级与有效期",
      "category": "账户"
    },
    "accounts_drefresh": {
      "label": "刷新 OAuth 凭证",
      "category": "账户"
    },
    "accounts_drecover": {
      "label": "清除账户错误与限流状态",
      "category": "账户"
    },
    "accounts_ddelete": {
      "label": "删除账户（影响共享分组）",
      "category": "账户"
    },
    "accounts_dgroups": {
      "label": "调整授权分组关联",
      "category": "账户"
    },
    "groups_dmodels_dread": {
      "label": "读取所选分组模型白名单与候选 ID",
      "category": "分组模型"
    },
    "groups_dmodels_dwrite": {
      "label": "编辑所选分组模型白名单（影响整个分组）",
      "category": "分组模型"
    },
    "proxies_dread": {
      "label": "读取代理 IP 候选（分页，不含认证凭证）",
      "category": "代理"
    },
    "models_ddefaults_dread": {
      "label": "读取默认白名单模型 ID（含默认映射）",
      "category": "模型"
    },
    "dashboard_dread": {
      "label": "读取全站经营报表",
      "category": "报表"
    },
    "users_dread": {
      "label": "读取全站用户资料（含联系方式）",
      "category": "用户"
    },
    "users_dbalance_dread": {
      "label": "读取全站用户余额及流水",
      "category": "财务"
    },
    "users_dbalance_dwrite": {
      "label": "增减任意用户余额",
      "category": "财务"
    },
    "users_dquotas_dread": {
      "label": "读取全站用户平台额度",
      "category": "用户"
    },
    "users_dquotas_dwrite": {
      "label": "修改与重置全站用户平台额度",
      "category": "用户"
    },
    "groups_dread": {
      "label": "读取所有分组配置",
      "category": "分组"
    },
    "usage_dread": {
      "label": "读取全站用量明细及用户用量",
      "category": "报表"
    },
    "payment_dread": {
      "label": "读取全站支付订单与报表",
      "category": "财务"
    },
    "payment_dcancel": {
      "label": "取消支付订单",
      "category": "财务"
    },
    "payment_dfulfill": {
      "label": "重新履约支付订单",
      "category": "财务"
    },
    "payment_drefund": {
      "label": "发起退款及确认退款结果",
      "category": "财务"
    },
    "payment_dplans_dread": {
      "label": "读取全部支付套餐",
      "category": "财务"
    },
    "payment_dplans_dwrite": {
      "label": "创建、修改和删除支付套餐",
      "category": "财务"
    },
    "subscriptions_dread": {
      "label": "读取全站订阅记录",
      "category": "订阅"
    },
    "subscriptions_dassign": {
      "label": "分配全站用户订阅",
      "category": "订阅"
    },
    "subscriptions_dextend": {
      "label": "延长订阅有效期",
      "category": "订阅"
    },
    "subscriptions_drevoke": {
      "label": "撤销订阅",
      "category": "订阅"
    },
    "subscriptions_drestore": {
      "label": "恢复订阅",
      "category": "订阅"
    },
    "subscriptions_dreset": {
      "label": "重置订阅额度",
      "category": "订阅"
    },
    "announcements_dread": {
      "label": "读取所有公告与阅读记录",
      "category": "内容"
    },
    "announcements_dwrite": {
      "label": "创建、发布与修改公告",
      "category": "内容"
    },
    "announcements_ddelete": {
      "label": "删除公告",
      "category": "内容"
    },
    "promo_dread": {
      "label": "读取优惠码及使用记录",
      "category": "财务"
    },
    "promo_dwrite": {
      "label": "创建与修改优惠码权益",
      "category": "财务"
    },
    "promo_ddelete": {
      "label": "删除优惠码",
      "category": "财务"
    },
    "ops_dread": {
      "label": "读取全站运维指标",
      "category": "监控"
    },
    "ops_derrors_dread": {
      "label": "读取全站请求错误详情（敏感）",
      "category": "监控"
    },
    "ops_derrors_dresolve": {
      "label": "标记请求错误处理状态",
      "category": "监控"
    },
    "ops_dalerts_dread": {
      "label": "读取全站告警规则与事件",
      "category": "监控"
    },
    "ops_dalerts_dwrite": {
      "label": "修改告警规则、状态与静默",
      "category": "监控"
    },
    "system_dread": {
      "label": "读取系统版本与可更新版本",
      "category": "系统"
    }
  },
  "listSeparator": "、"
}
