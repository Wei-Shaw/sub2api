export default {

  // Subscription Progress (Header component)
  subscriptionProgress: {
    title: '我的订阅',
    viewDetails: '查看订阅详情',
    activeCount: '{count} 个有效订阅',
    daily: '每日',
    weekly: '每周',
    monthly: '每月',
    daysRemaining: '剩余 {days} 天',
    expired: '已过期',
    expiresToday: '今天到期',
    expiresTomorrow: '明天到期',
    viewAll: '查看全部订阅',
    noSubscriptions: '暂无有效订阅',
    unlimited: '无限制'
  },

  // Version Badge
  version: {
    currentVersion: '当前版本',
    latestVersion: '最新版本',
    upToDate: '已是最新版本',
    updateAvailable: '有新版本可用！',
    releaseNotes: '更新日志',
    noReleaseNotes: '暂无更新日志',
    viewUpdate: '查看更新',
    viewRelease: '查看发布',
    viewChangelog: '查看更新日志',
    refresh: '刷新',
    sourceMode: '源码构建',
    sourceModeHint: '源码构建请使用 git pull 更新',
    updateNow: '立即更新',
    updating: '正在更新...',
    updateComplete: '更新完成',
    updateFailed: '更新失败',
    restartRequired: '请重启服务以应用更新',
    restartNow: '立即重启',
    restarting: '正在重启...',
    retry: '重试',
    rollback: '版本回退',
    rollbackSelectVersion: '选择要回退到的版本（近 3 个版本）',
    rollbackConfirm: '回退到 {version}',
    rollbackWarning: '回退将下载所选版本并替换当前程序，完成后需重启服务',
    rollingBack: '正在回退...',
    rollbackComplete: '回退完成',
    rollbackFailed: '回退失败',
    manualRollbackCommand: '手动回退方式',
    copyCommand: '复制',
    copied: '已复制',
    noRollbackVersions: '暂无可回退的版本',
    loadVersionsFailed: '获取版本列表失败',
    rollbackSourceHint: '源码构建不支持在线回退',
    deployScript: '脚本部署',
    deployDocker: 'Docker',
    dockerEditCompose: '修改 docker-compose.yml 中的镜像版本',
    dockerRecreate: '重新创建容器'
  },

  // Recharge / Subscription Page
  purchase: {
    title: '充值/订阅',
    description: '通过内嵌页面完成充值/订阅',
    openInNewTab: '新窗口打开',
    notEnabledTitle: '该功能未开启',
    notEnabledDesc: '管理员暂未开启充值/订阅入口，请联系管理员。',
    notConfiguredTitle: '充值/订阅链接未配置',
    notConfiguredDesc: '管理员已开启入口，但尚未配置充值/订阅链接，请联系管理员。'
  },

  // Custom Page (iframe embed)
  customPage: {
    title: '自定义页面',
    openInNewTab: '新窗口打开',
    notFoundTitle: '页面不存在',
    notFoundDesc: '该自定义页面不存在或已被删除。',
    notConfiguredTitle: '页面链接未配置',
    notConfiguredDesc: '该自定义页面的 URL 未正确配置。',
    tableOfContents: '目录',
    copyCode: '复制',
    copiedCode: '已复制',
    copyCodeFailed: '失败'
  },

  // Announcements Page
  announcements: {
    title: '公告',
    description: '查看系统公告',
    unreadOnly: '仅显示未读',
    markRead: '标记已读',
    markAllRead: '全部已读',
    viewAll: '查看全部公告',
    markedAsRead: '已标记为已读',
    allMarkedAsRead: '所有公告已标记为已读',
    newCount: '有 {count} 条新公告',
    readAt: '已读时间',
    read: '已读',
    unread: '未读',
    startsAt: '开始时间',
    endsAt: '结束时间',
    empty: '暂无公告',
    emptyUnread: '暂无未读公告',
    total: '条公告',
    emptyDescription: '暂时没有任何系统公告',
    readStatus: '您已阅读此公告',
    markReadHint: '点击"已读"标记此公告'
  },

  // User Subscriptions Page
  userSubscriptions: {
    title: '我的订阅',
    description: '查看您的订阅计划和用量',
    noActiveSubscriptions: '暂无有效订阅',
    noActiveSubscriptionsDesc: '您没有任何有效订阅。请联系管理员获取订阅。',
    failedToLoad: '加载订阅失败',
    status: {
      active: '有效',
      expired: '已过期',
      revoked: '已撤销'
    },
    usage: '用量',
    expires: '到期时间',
    noExpiration: '无到期时间',
    unlimited: '无限制',
    unlimitedDesc: '该订阅无用量限制',
    daily: '每日',
    weekly: '每周',
    monthly: '每月',
    daysRemaining: '剩余 {days} 天',
    expiresOn: '{date} 到期',
    resetIn: '{time} 后重置',
    quotaEndsIn: '额度将在 {time} 后结束',
    windowNotActive: '等待首次使用',
    usageOf: '已用 {used} / {limit}'
  },

  // Onboarding Tour
  onboarding: {
    restartTour: '重新查看新手引导',
    dontShowAgain: '不再提示',
    dontShowAgainTitle: '永久关闭新手引导',
    confirmDontShow: '确定不再显示新手引导吗？\n\n您可以随时在右上角头像菜单中重新开启。',
    confirmExit: '确定要退出新手引导吗？您可以随时在右上角菜单重新开始。',
    interactiveHint: '按 Enter 或点击继续',
    navigation: {
      flipPage: '翻页',
      exit: '退出'
    },
    // Admin tour steps
    admin: {
      welcome: {
        title: '👋 欢迎使用 Sub2API',
        description:
          '<div style="line-height: 1.8;"><p style="margin-bottom: 16px;">Sub2API 是一个强大的 AI 服务中转平台，让您轻松管理和分发 AI 服务。</p><p style="margin-bottom: 12px;"><b>🎯 核心功能：</b></p><ul style="margin-left: 20px; margin-bottom: 16px;"><li>📦 <b>分组管理</b> - 创建不同的服务套餐（VIP、免费试用等）</li><li>🔗 <b>账号池</b> - 连接多个上游 AI 服务商账号</li><li>🔑 <b>密钥分发</b> - 为用户生成独立的 API Key</li><li>💰 <b>计费管理</b> - 灵活的费率和配额控制</li></ul><p style="color: #10b981; font-weight: 600;">接下来，我们将用 3 分钟带您完成首次配置 →</p></div>',
        nextBtn: '开始配置 🚀',
        prevBtn: '跳过'
      },
      groupManage: {
        title: '📦 第一步：分组管理',
        description:
          '<div style="line-height: 1.7;"><p style="margin-bottom: 12px;"><b>什么是分组？</b></p><p style="margin-bottom: 12px;">分组是 Sub2API 的核心概念，它就像一个"服务套餐"：</p><ul style="margin-left: 20px; margin-bottom: 12px; font-size: 13px;"><li>🎯 每个分组可以包含多个上游账号</li><li>💰 每个分组有独立的计费倍率</li><li>👥 可以设置为公开或专属分组</li></ul><p style="margin-top: 12px; padding: 8px 12px; background: #f0fdf4; border-left: 3px solid #10b981; border-radius: 4px; font-size: 13px;"><b>💡 示例：</b>您可以创建"VIP专线"（高倍率）和"免费试用"（低倍率）两个分组</p><p style="margin-top: 16px; color: #10b981; font-weight: 600;">👉 点击左侧的"分组管理"开始</p></div>'
      },
      createGroup: {
        title: '➕ 创建新分组',
        description:
          '<div style="line-height: 1.7;"><p style="margin-bottom: 12px;">现在让我们创建第一个分组。</p><p style="padding: 8px 12px; background: #eff6ff; border-left: 3px solid #3b82f6; border-radius: 4px; font-size: 13px; margin-bottom: 12px;"><b>📝 提示：</b>建议先创建一个测试分组，熟悉流程后再创建正式分组</p><p style="color: #10b981; font-weight: 600;">👉 点击"创建分组"按钮</p></div>'
      },
      groupName: {
        title: '✏️ 1. 分组名称',
        description:
          '<div style="line-height: 1.7;"><p style="margin-bottom: 12px;">为您的分组起一个易于识别的名称。</p><div style="padding: 8px 12px; background: #f0fdf4; border-left: 3px solid #10b981; border-radius: 4px; font-size: 13px; margin-bottom: 12px;"><b>💡 命名建议：</b><ul style="margin: 8px 0 0 16px;"><li>"测试分组" - 用于测试</li><li>"VIP专线" - 高质量服务</li><li>"免费试用" - 体验版</li></ul></div><p style="font-size: 13px; color: #6b7280;">填写完成后点击"下一步"继续</p></div>',
        nextBtn: '下一步'
      },
      groupPlatform: {
        title: '🤖 2. 选择平台',
        description:
          '<div style="line-height: 1.7;"><p style="margin-bottom: 12px;">选择该分组支持的 AI 平台。</p><div style="padding: 8px 12px; background: #eff6ff; border-left: 3px solid #3b82f6; border-radius: 4px; font-size: 13px; margin-bottom: 12px;"><b>📌 平台说明：</b><ul style="margin: 8px 0 0 16px;"><li><b>Anthropic</b> - Claude 系列模型</li><li><b>OpenAI</b> - GPT 系列模型</li><li><b>Google</b> - Gemini 系列模型</li></ul></div><p style="font-size: 13px; color: #6b7280;">一个分组只能选择一个平台</p></div>',
        nextBtn: '下一步'
      },
      groupMultiplier: {
        title: '💰 3. 费率倍数',
        description:
          '<div style="line-height: 1.7;"><p style="margin-bottom: 12px;">设置该分组的计费倍率，控制用户的实际扣费。</p><div style="padding: 8px 12px; background: #fef3c7; border-left: 3px solid #f59e0b; border-radius: 4px; font-size: 13px; margin-bottom: 12px;"><b>⚙️ 计费规则：</b><ul style="margin: 8px 0 0 16px;"><li><b>1.0</b> - 原价计费（成本价）</li><li><b>1.5</b> - 用户消耗 $1，扣除 $1.5</li><li><b>2.0</b> - 用户消耗 $1，扣除 $2</li><li><b>0.8</b> - 补贴模式（亏本运营）</li></ul></div><p style="font-size: 13px; color: #6b7280;">建议测试分组设置为 1.0</p></div>',
        nextBtn: '下一步'
      },
      groupExclusive: {
        title: '🔒 4. 专属分组（可选）',
        description:
          '<div style="line-height: 1.7;"><p style="margin-bottom: 12px;">控制分组的可见性和访问权限。</p><div style="padding: 8px 12px; background: #eff6ff; border-left: 3px solid #3b82f6; border-radius: 4px; font-size: 13px; margin-bottom: 12px;"><b>🔐 权限说明：</b><ul style="margin: 8px 0 0 16px;"><li><b>关闭</b> - 公开分组，所有用户可见</li><li><b>开启</b> - 专属分组，仅指定用户可见</li></ul></div><p style="padding: 8px 12px; background: #f0fdf4; border-left: 3px solid #10b981; border-radius: 4px; font-size: 13px;"><b>💡 使用场景：</b>VIP 用户专属、内部测试、特殊客户等</p></div>',
        nextBtn: '下一步'
      },
      groupSubmit: {
        title: '✅ 保存分组',
        description:
          '<div style="line-height: 1.7;"><p style="margin-bottom: 12px;">确认信息无误后，点击创建按钮保存分组。</p><p style="padding: 8px 12px; background: #fef3c7; border-left: 3px solid #f59e0b; border-radius: 4px; font-size: 13px; margin-bottom: 12px;"><b>⚠️ 注意：</b>分组创建后，平台类型不可修改，其他信息可以随时编辑</p><p style="padding: 8px 12px; background: #f0fdf4; border-left: 3px solid #10b981; border-radius: 4px; font-size: 13px;"><b>📌 下一步：</b>创建成功后，我们将添加上游账号到这个分组</p><p style="margin-top: 12px; color: #10b981; font-weight: 600;">👉 点击"创建"按钮</p></div>'
      },
      accountManage: {
        title: '🔗 第二步：添加账号',
        description:
          '<div style="line-height: 1.7;"><p style="margin-bottom: 12px;"><b>太棒了！分组已创建成功 🎉</b></p><p style="margin-bottom: 12px;">现在需要添加上游 AI 服务商的账号，让分组能够实际提供服务。</p><div style="padding: 8px 12px; background: #eff6ff; border-left: 3px solid #3b82f6; border-radius: 4px; font-size: 13px; margin-bottom: 12px;"><b>🔑 账号的作用：</b><ul style="margin: 8px 0 0 16px;"><li>连接到上游 AI 服务（Claude、GPT 等）</li><li>一个分组可以包含多个账号（负载均衡）</li><li>支持 OAuth 和 Session Key 两种方式</li></ul></div><p style="margin-top: 16px; color: #10b981; font-weight: 600;">👉 点击左侧的"账号管理"</p></div>'
      },
      createAccount: {
        title: '➕ 添加新账号',
        description:
          '<div style="line-height: 1.7;"><p style="margin-bottom: 12px;">点击按钮开始添加您的第一个上游账号。</p><p style="padding: 8px 12px; background: #f0fdf4; border-left: 3px solid #10b981; border-radius: 4px; font-size: 13px;"><b>💡 提示：</b>建议使用 OAuth 方式，更安全且无需手动提取密钥</p><p style="margin-top: 12px; color: #10b981; font-weight: 600;">👉 点击"添加账号"按钮</p></div>'
      },
      accountName: {
        title: '✏️ 1. 账号名称',
        description:
          '<div style="line-height: 1.7;"><p style="margin-bottom: 12px;">为账号设置一个便于识别的名称。</p><p style="padding: 8px 12px; background: #f0fdf4; border-left: 3px solid #10b981; border-radius: 4px; font-size: 13px;"><b>💡 命名建议：</b>"Claude主账号"、"GPT备用1"、"测试账号" 等</p></div>',
        nextBtn: '下一步'
      },
      accountPlatform: {
        title: '🤖 2. 选择平台',
        description:
          '<div style="line-height: 1.7;"><p style="margin-bottom: 12px;">选择该账号对应的服务商平台。</p><p style="padding: 8px 12px; background: #fef3c7; border-left: 3px solid #f59e0b; border-radius: 4px; font-size: 13px;"><b>⚠️ 重要：</b>平台必须与刚才创建的分组平台一致</p></div>',
        nextBtn: '下一步'
      },
      accountType: {
        title: '🔐 3. 授权方式',
        description:
          '<div style="line-height: 1.7;"><p style="margin-bottom: 12px;">选择账号的授权方式。</p><div style="padding: 8px 12px; background: #f0fdf4; border-left: 3px solid #10b981; border-radius: 4px; font-size: 13px; margin-bottom: 12px;"><b>✅ 推荐：OAuth 方式</b><ul style="margin: 8px 0 0 16px;"><li>无需手动提取密钥</li><li>更安全，支持自动刷新</li><li>适用于 Claude Code、ChatGPT OAuth</li></ul></div><div style="padding: 8px 12px; background: #eff6ff; border-left: 3px solid #3b82f6; border-radius: 4px; font-size: 13px;"><b>📌 Session Key 方式</b><ul style="margin: 8px 0 0 16px;"><li>需要手动从浏览器提取</li><li>可能需要定期更新</li><li>适用于不支持 OAuth 的平台</li></ul></div></div>',
        nextBtn: '下一步'
      },
      accountPriority: {
        title: '⚖️ 4. 优先级（可选）',
        description:
          '<div style="line-height: 1.7;"><p style="margin-bottom: 12px;">设置账号的调用优先级。</p><div style="padding: 8px 12px; background: #eff6ff; border-left: 3px solid #3b82f6; border-radius: 4px; font-size: 13px; margin-bottom: 12px;"><b>📊 优先级规则：</b><ul style="margin: 8px 0 0 16px;"><li>数字越小，优先级越高</li><li>系统优先使用低数值账号</li><li>相同优先级则随机选择</li></ul></div><p style="padding: 8px 12px; background: #f0fdf4; border-left: 3px solid #10b981; border-radius: 4px; font-size: 13px;"><b>💡 使用场景：</b>主账号设置低数值，备用账号设置高数值</p></div>',
        nextBtn: '下一步'
      },
      accountGroups: {
        title: '🎯 5. 分配分组',
        description:
          '<div style="line-height: 1.7;"><p style="margin-bottom: 12px;"><b>关键步骤！</b>将账号分配到刚才创建的分组。</p><div style="padding: 8px 12px; background: #fee2e2; border-left: 3px solid #ef4444; border-radius: 4px; font-size: 13px; margin-bottom: 12px;"><b>⚠️ 重要提醒：</b><ul style="margin: 8px 0 0 16px;"><li>必须勾选至少一个分组</li><li>未分配分组的账号无法使用</li><li>一个账号可以分配给多个分组</li></ul></div><p style="padding: 8px 12px; background: #f0fdf4; border-left: 3px solid #10b981; border-radius: 4px; font-size: 13px;"><b>💡 提示：</b>请勾选刚才创建的测试分组</p></div>',
        nextBtn: '下一步'
      },
      accountSubmit: {
        title: '✅ 保存账号',
        description:
          '<div style="line-height: 1.7;"><p style="margin-bottom: 12px;">确认信息无误后，点击保存按钮。</p><div style="padding: 8px 12px; background: #eff6ff; border-left: 3px solid #3b82f6; border-radius: 4px; font-size: 13px; margin-bottom: 12px;"><b>📌 OAuth 授权流程：</b><ul style="margin: 8px 0 0 16px;"><li>点击保存后会跳转到服务商页面</li><li>在服务商页面完成登录授权</li><li>授权成功后自动返回</li></ul></div><p style="padding: 8px 12px; background: #f0fdf4; border-left: 3px solid #10b981; border-radius: 4px; font-size: 13px;"><b>📌 下一步：</b>账号添加成功后，我们将创建 API 密钥</p><p style="margin-top: 12px; color: #10b981; font-weight: 600;">👉 点击"保存"按钮</p></div>'
      },
      keyManage: {
        title: '🔑 第三步：生成密钥',
        description:
          '<div style="line-height: 1.7;"><p style="margin-bottom: 12px;"><b>恭喜！账号配置完成 🎉</b></p><p style="margin-bottom: 12px;">最后一步，生成 API Key 来测试服务是否正常工作。</p><div style="padding: 8px 12px; background: #eff6ff; border-left: 3px solid #3b82f6; border-radius: 4px; font-size: 13px; margin-bottom: 12px;"><b>🔑 API Key 的作用：</b><ul style="margin: 8px 0 0 16px;"><li>用于调用 AI 服务的凭证</li><li>每个 Key 绑定一个分组</li><li>可以设置配额和有效期</li><li>支持独立的使用统计</li></ul></div><p style="margin-top: 16px; color: #10b981; font-weight: 600;">👉 点击左侧的"API 密钥"</p></div>'
      },
      createKey: {
        title: '➕ 创建密钥',
        description:
          '<div style="line-height: 1.7;"><p style="margin-bottom: 12px;">点击按钮创建您的第一个 API Key。</p><p style="padding: 8px 12px; background: #f0fdf4; border-left: 3px solid #10b981; border-radius: 4px; font-size: 13px;"><b>💡 提示：</b>创建后请立即复制保存，密钥只显示一次</p><p style="margin-top: 12px; color: #10b981; font-weight: 600;">👉 点击"创建密钥"按钮</p></div>'
      },
      keyName: {
        title: '✏️ 1. 密钥名称',
        description:
          '<div style="line-height: 1.7;"><p style="margin-bottom: 12px;">为密钥设置一个便于管理的名称。</p><p style="padding: 8px 12px; background: #f0fdf4; border-left: 3px solid #10b981; border-radius: 4px; font-size: 13px;"><b>💡 命名建议：</b>"测试密钥"、"生产环境"、"移动端" 等</p></div>',
        nextBtn: '下一步'
      },
      keyGroup: {
        title: '🎯 2. 选择分组',
        description:
          '<div style="line-height: 1.7;"><p style="margin-bottom: 12px;">选择刚才配置好的分组。</p><div style="padding: 8px 12px; background: #eff6ff; border-left: 3px solid #3b82f6; border-radius: 4px; font-size: 13px; margin-bottom: 12px;"><b>📌 分组决定：</b><ul style="margin: 8px 0 0 16px;"><li>该密钥可以使用哪些账号</li><li>计费倍率是多少</li><li>是否为专属密钥</li></ul></div><p style="padding: 8px 12px; background: #f0fdf4; border-left: 3px solid #10b981; border-radius: 4px; font-size: 13px;"><b>💡 提示：</b>选择刚才创建的测试分组</p></div>',
        nextBtn: '下一步'
      },
      keySubmit: {
        title: '🎉 生成并复制',
        description:
          '<div style="line-height: 1.7;"><p style="margin-bottom: 12px;">点击创建后，系统会生成完整的 API Key。</p><div style="padding: 8px 12px; background: #fee2e2; border-left: 3px solid #ef4444; border-radius: 4px; font-size: 13px; margin-bottom: 12px;"><b>⚠️ 重要提醒：</b><ul style="margin: 8px 0 0 16px;"><li>密钥只显示一次，请立即复制</li><li>丢失后需要重新生成</li><li>妥善保管，不要泄露给他人</li></ul></div><div style="padding: 8px 12px; background: #f0fdf4; border-left: 3px solid #10b981; border-radius: 4px; font-size: 13px; margin-bottom: 12px;"><b>🚀 下一步：</b><ul style="margin: 8px 0 0 16px;"><li>复制生成的 sk-xxx 密钥</li><li>在支持 OpenAI 接口的客户端中使用</li><li>开始体验 AI 服务！</li></ul></div><p style="margin-top: 12px; color: #10b981; font-weight: 600;">👉 点击"创建"按钮</p></div>'
      }
    },
    // User tour steps
    user: {
      welcome: {
        title: '👋 欢迎使用 Sub2API',
        description:
          '<div style="line-height: 1.8;"><p style="margin-bottom: 16px;">您好！欢迎来到 Sub2API AI 服务平台。</p><p style="margin-bottom: 12px;"><b>🎯 快速开始：</b></p><ul style="margin-left: 20px; margin-bottom: 16px;"><li>🔑 创建 API 密钥</li><li>📋 复制密钥到您的应用</li><li>🚀 开始使用 AI 服务</li></ul><p style="color: #10b981; font-weight: 600;">只需 1 分钟，让我们开始吧 →</p></div>',
        nextBtn: '开始 🚀',
        prevBtn: '跳过'
      },
      keyManage: {
        title: '🔑 API 密钥管理',
        description:
          '<div style="line-height: 1.7;"><p style="margin-bottom: 12px;">在这里管理您的所有 API 访问密钥。</p><p style="padding: 8px 12px; background: #eff6ff; border-left: 3px solid #3b82f6; border-radius: 4px; font-size: 13px;"><b>📌 什么是 API 密钥？</b><br/>API 密钥是您访问 AI 服务的凭证，就像一把钥匙，让您的应用能够调用 AI 能力。</p><p style="margin-top: 12px; color: #10b981; font-weight: 600;">👉 点击进入密钥页面</p></div>'
      },
      createKey: {
        title: '➕ 创建新密钥',
        description:
          '<div style="line-height: 1.7;"><p style="margin-bottom: 12px;">点击按钮创建您的第一个 API 密钥。</p><p style="padding: 8px 12px; background: #f0fdf4; border-left: 3px solid #10b981; border-radius: 4px; font-size: 13px;"><b>💡 提示：</b>创建后密钥只显示一次，请务必复制保存</p><p style="margin-top: 12px; color: #10b981; font-weight: 600;">👉 点击"创建密钥"</p></div>'
      },
      keyName: {
        title: '✏️ 密钥名称',
        description:
          '<div style="line-height: 1.7;"><p style="margin-bottom: 12px;">为密钥起一个便于识别的名称。</p><p style="padding: 8px 12px; background: #f0fdf4; border-left: 3px solid #10b981; border-radius: 4px; font-size: 13px;"><b>💡 示例：</b>"我的第一个密钥"、"测试用" 等</p></div>',
        nextBtn: '下一步'
      },
      keyGroup: {
        title: '🎯 选择分组',
        description:
          '<div style="line-height: 1.7;"><p style="margin-bottom: 12px;">选择管理员为您分配的服务分组。</p><p style="padding: 8px 12px; background: #eff6ff; border-left: 3px solid #3b82f6; border-radius: 4px; font-size: 13px;"><b>📌 分组说明：</b><br/>不同分组可能有不同的服务质量和计费标准，请根据需要选择。</p></div>',
        nextBtn: '下一步'
      },
      keySubmit: {
        title: '🎉 完成创建',
        description:
          '<div style="line-height: 1.7;"><p style="margin-bottom: 12px;">点击确认创建您的 API 密钥。</p><div style="padding: 8px 12px; background: #fee2e2; border-left: 3px solid #ef4444; border-radius: 4px; font-size: 13px; margin-bottom: 12px;"><b>⚠️ 重要：</b><ul style="margin: 8px 0 0 16px;"><li>创建后请立即复制密钥（sk-xxx）</li><li>密钥只显示一次，丢失需重新生成</li></ul></div><p style="padding: 8px 12px; background: #f0fdf4; border-left: 3px solid #10b981; border-radius: 4px; font-size: 13px;"><b>🚀 如何使用：</b><br/>将密钥配置到支持 OpenAI 接口的任何客户端（如 ChatBox、OpenCat 等），即可开始使用！</p><p style="margin-top: 12px; color: #10b981; font-weight: 600;">👉 点击"创建"按钮</p></div>'
      }
    }
  },

  // Payment System
  payment: {
    title: '充值/订阅',
    amountLabel: '充值金额',
    paymentAmount: '支付金额',
    creditedBalance: '到账余额',
    quickAmounts: '快捷金额',
    customAmount: '自定义金额',
    enterAmount: '输入金额',
    paymentMethod: '支付方式',
    fee: '手续费',
    actualPay: '实付金额',
    createOrder: '确认支付',
    methods: {
      easypay: '易支付',
      alipay: '支付宝',
      wxpay: '微信支付',
      stripe: 'Stripe',
      airwallex: 'Airwallex',
      card: '银行卡',
      link: 'Link',
      alipay_direct: '支付宝（直连）',
      wxpay_direct: '微信支付（直连）',
    },
    status: {
      pending: '待支付',
      paid: '已支付',
      recharging: '充值中',
      completed: '已完成',
      expired: '已过期',
      cancelled: '已取消',
      failed: '失败',
      refund_requested: '退款申请中',
      refunding: '退款中',
      refund_pending: '退款处理中',
      refunded: '已退款',
      partially_refunded: '部分退款',
      refund_failed: '退款失败',
    },
    qr: {
      scanToPay: '请扫码支付',
      scanAlipay: '支付宝扫码支付',
      scanWxpay: '微信扫码支付',
      scanAlipayHint: '请使用手机打开支付宝，扫描二维码完成支付',
      scanWxpayHint: '请使用手机打开微信，扫描二维码完成支付',
      payInNewWindow: '请在新窗口中完成支付',
      payInNewWindowHint: '支付页面已在新窗口打开，请在新窗口中完成支付后返回此页面',
      openPayWindow: '重新打开支付页面',
      expiresIn: '剩余支付时间',
      expired: '订单已过期',
      expiredDesc: '订单已超时，请重新创建订单',
      cancelled: '订单已取消',
      cancelledDesc: '您已取消本次支付',
      waitingPayment: '等待支付...',
      cancelOrder: '取消订单',
    },
    orders: {
      title: '我的订单',
      empty: '暂无订单',
      orderId: '订单 ID',
      orderNo: '订单编号',
      amount: '金额',
      payAmount: '实付',
      creditedAmount: '到账金额',
      fee: '手续费',
      baseAmount: '充值金额',
      includedInPayAmount: '已含在实付金额中',
      status: '状态',
      paymentMethod: '支付方式',
      createdAt: '创建时间',
      cancel: '取消订单',
      userId: '用户 ID',
      orderType: '订单类型',
      actions: '操作',
      requestRefund: '申请退款',
    },
    result: {
      success: '支付成功',
      subscriptionSuccess: '订阅成功',
      processing: '支付处理中',
      processingHint: '支付结果仍在确认中，页面会自动刷新。',
      failed: '支付失败',
      backToRecharge: '返回充值',
      viewOrders: '查看订单',
    },
    currentBalance: '当前余额',
    groupFallback: '分组 #{id}',
    rechargeAccount: '充值账户',
    activeSubscription: '当前订阅',
    noActiveSubscription: '暂无有效订阅',
    tabTopUp: '充值',
    tabSubscribe: '订阅',
    noPlans: '暂无可用订阅套餐',
    notAvailable: '充值功能暂未开放',
    confirmSubscription: '确认订阅',
    confirmCancel: '确定要取消此订单吗？',
    amountTooLow: '最低金额为 {min}',
    amountTooHigh: '最高金额为 {max}',
    amountNoMethod: '该金额没有可用的支付方式',
    rechargeRatePreview: '当前倍率：1 CNY = {usd} USD',
    refundReason: '退款原因',
    refundReasonPlaceholder: '请描述您的退款原因',
    stripeLoadFailed: '支付组件加载失败，请刷新页面重试',
    stripeMissingParams: '缺少订单ID或支付密钥',
    stripeNotConfigured: 'Stripe 未配置',
    airwallexLoadFailed: 'Airwallex 支付组件加载失败，请刷新页面重试',
    airwallexMissingParams: '缺少 Airwallex 支付参数',
    errors: {
      tooManyPending: '待支付订单过多（最多 {max} 个），请先完成或取消现有订单',
      cancelRateLimited: '取消订单过于频繁，请稍后再试',
      wechatH5NotAuthorized: '当前商户未开通微信 H5 支付，请在微信中打开当前页面继续支付。',
      wechatPaymentMpNotConfigured: '当前站点未完成公众号/JSAPI 支付配置，暂时无法在微信内直接拉起支付。',
      wechatJsapiUnavailable: '当前环境未能拉起微信支付，请确认正在微信内打开本页后重试。',
      wechatJsapiFailed: '微信支付未完成，请重新拉起支付或改用扫码支付。',
      wechatUnavailable: '当前微信支付暂不可用，请稍后重试。',
      wechatOpenInWeChatHint: '请复制当前页面链接到微信内打开，或直接改用电脑端微信扫码支付。',
      wechatScanOnDesktopHint: '电脑端请直接使用微信扫一扫完成支付；移动端请在微信内打开当前页面。',
      wechatSwitchBrowserHint: '请改用电脑端微信扫码，或在外部浏览器重新打开本页后再试。',
      mobilePaymentFallbackToQr: '当前商户未开通移动支付，已自动切换为扫码支付。',
      alipayDesktopUnavailable: '当前支付宝桌面支付未成功生成二维码。',
      alipayDesktopQrHint: '电脑端支付宝应展示扫码单，请刷新后重试，或确认浏览器未拦截当前支付页。',
      alipayMobileUnavailable: '当前页面未成功跳转到支付宝。',
      alipayMobileOpenHint: '请允许当前页面打开支付宝 App，或改用系统浏览器重新发起支付。',
      // Structured error codes (reason strings from backend ApplicationError)
      PAYMENT_DISABLED: '支付系统已关闭',
      USER_INACTIVE: '账号已被禁用',
      BALANCE_PAYMENT_DISABLED: '余额充值功能已关闭',
      INVALID_AMOUNT: '金额无效',
      INVALID_INPUT: '参数有误',
      PLAN_NOT_AVAILABLE: '套餐不存在或已下架',
      GROUP_NOT_FOUND: '订阅分组不可用',
      GROUP_TYPE_MISMATCH: '分组类型不是订阅类型',
      TOO_MANY_PENDING: '待支付订单过多（最多 {max} 个），请先完成或取消现有订单',
      DAILY_LIMIT_EXCEEDED: '今日充值已达上限，剩余额度 {remaining}',
      PAYMENT_GATEWAY_ERROR: '支付方式不可用',
      NO_AVAILABLE_INSTANCE: '暂无可用的支付通道',
      PAYMENT_PROVIDER_MISCONFIGURED: '支付通道配置错误，请联系管理员',
      WXPAY_CONFIG_MISSING_KEY: '微信支付配置缺少必填项：{key}',
      WXPAY_CONFIG_INVALID_KEY_LENGTH: '微信支付 {key} 长度错误，应为 {expected} 字节（实际 {actual}）',
      WXPAY_CONFIG_INVALID_KEY: '微信支付 {key} 格式错误，请确认复制了完整的 PEM 内容',
      PENDING_ORDERS: '该服务商有未完成的订单，请等待订单完成后再操作',
      PAYMENT_PROVIDER_CONFLICT: '该支付方式已有其他启用中的服务商实例，请先停用后再继续。',
      CANCEL_RATE_LIMITED: '取消订单过于频繁，请稍后再试',
      NOT_FOUND: '订单不存在',
      FORBIDDEN: '无权限操作此订单',
      CONFLICT: '订单状态已变更，请刷新',
      INVALID_ORDER_TYPE: '仅余额订单可申请退款',
      INVALID_STATUS: '当前订单状态不允许此操作',
      BALANCE_NOT_ENOUGH: '退款金额超过余额',
      REFUND_AMOUNT_EXCEEDED: '退款金额超过充值金额',
      REFUND_FAILED: '退款失败',
    },
    airwallexPay: 'Airwallex 支付',
    stripePay: '立即支付',
    stripeSuccessProcessing: '支付成功，正在处理订单...',
    stripePopup: {
      redirecting: '正在跳转到支付页面...',
      loadingQr: '正在获取微信支付二维码...',
      timeout: '等待支付凭证超时，请重试',
      qrFailed: '未能获取微信支付二维码',
    },
    subscribeNow: '立即开通',
    renewNow: '续费',
    selectPlan: '选择套餐',
    planFeatures: '功能特性',
    planCard: {
      rate: '倍率',
      peakRate: '高峰倍率',
      dailyLimit: '日限额',
      weeklyLimit: '周限额',
      monthlyLimit: '月限额',
      quota: '配额',
      unlimited: '无限制',
      models: '模型',
    },
    days: '天',
    weeks: '周',
    months: '个月',
    years: '年',
    oneMonth: '1 个月',
    oneYear: '1 年',
    perMonth: '月',
    perYear: '年',
    admin: {
      tabs: {
        overview: '概览',
        orders: '订单管理',
        channels: '支付渠道',
        plans: '订阅套餐',
      },
      todayRevenue: '今日收入',
      totalRevenue: '总收入',
      todayOrders: '今日订单',
      orderCount: '订单数',
      avgAmount: '平均金额',
      revenue: '收入',
      dailyRevenue: '每日收入',
      paymentDistribution: '支付方式分布',
      colUser: '用户',
      topUsers: '消费排行',
      noData: '暂无数据',
      days: '天',
      weeks: '周',
      months: '月',
      searchOrders: '搜索订单...',
      allStatuses: '全部状态',
      allPaymentTypes: '全部支付方式',
      allOrderTypes: '全部订单类型',
      orderDetail: '订单详情',
      orderType: '订单类型',
      orders: '订单',
      balanceOrder: '余额充值',
      subscriptionOrder: '订阅',
      paidAt: '支付时间',
      completedAt: '完成时间',
      expiresAt: '过期时间',
      feeRate: '手续费率',
      refund: '退款',
      refundOrder: '退款订单',
      refundAmount: '退款金额',
      maxRefundable: '最大可退金额',
      refundReason: '退款原因',
      refundReasonPlaceholder: '请输入退款原因',
      confirmRefund: '确认退款',
      refundSuccess: '退款成功',
      refundPending: '退款处理中，待网关确认',
      queryRefundStatus: '查询退款状态',
      refundInfo: '退款信息',
      refundEnabled: '允许退款',
      allowUserRefund: '允许用户退款',
      alreadyRefunded: '已退款',
      deductBalance: '扣除余额',
      deductBalanceHint: '从用户余额中扣回充值金额',
      userBalance: '用户余额',
      orderAmount: '订单金额',
      insufficientBalance: '余额不足，将扣至 $0',
      noDeduction: '将不扣除用户余额',
      forceRefund: '强制退款（忽略余额检查）',
      orderCancelled: '订单已取消',
      retry: '重试',
      retrySuccess: '重试成功',
      approveRefund: '批准退款',
      retryRefund: '重试退款',
      refundRequestInfo: '退款申请信息',
      refundRequestedAt: '申请时间',
      refundRequestedBy: '申请人',
      refundRequestReason: '申请原因',
      auditLogs: '操作日志',
      operator: '操作人',
      channelName: '渠道名称',
      channelDescription: '渠道描述',
      createChannel: '创建渠道',
      editChannel: '编辑渠道',
      deleteChannel: '删除渠道',
      deleteChannelConfirm: '确定要删除此渠道吗？',
      planName: '套餐名称',
      planDescription: '套餐描述',
      createPlan: '创建套餐',
      editPlan: '编辑套餐',
      deletePlan: '删除套餐',
      deletePlanConfirm: '确定要删除此套餐吗？',
      originalPrice: '原价',
      price: '价格',
      currency: '币种标注',
      currencyPlaceholder: '如 USD / NZD / CNY',
      currencyHint: '仅用于价格展示的 ISO 三字母币种码，留空不展示，不影响实际扣款',
      subscriptionCnyPayPreview: 'CNY 通道实扣预览：{amount}',
      subscriptionCnyPayPreviewWithFee: '（含 {feeRate}% 手续费：{total}）',
      validity: '有效期',
      validityUnit: '有效期单位',
      sortOrder: '排序',
      forSale: '上架状态',
      onSale: '上架',
      offSale: '下架',
      group: '分组',
      groupId: '分组 ID',
      features: '功能特性',
      featuresHint: '每行一个特性',
      featuresPlaceholder: '输入套餐特性...',
      providerManagement: '服务商管理',
      providerManagementDesc: '管理支付服务商实例',
      createProvider: '创建服务商',
      editProvider: '编辑服务商',
      deleteProvider: '删除服务商',
      deleteProviderConfirm: '确定要删除此服务商吗？',
      providerName: '服务商名称',
      providerKey: '服务商标识',
      selectProviderKey: '选择服务商标识',
      providerConfig: '服务商配置',
      noProviders: '暂无服务商',
      noProvidersHint: '创建一个服务商实例以开始接受支付',
      supportedTypes: '支持的支付方式',
      supportedTypesHint: '选择此服务商支持的支付方式',
      rateMultiplier: '费率倍数',
      dashboardTitle: '支付概览',
      dashboardDesc: '充值订单统计与分析',
      daySuffix: '天',
      paymentConfigTitle: '支付配置',
      paymentConfigDesc: '管理支付服务商与相关设置',
      plansPageTitle: '订阅套餐管理',
      plansPageDesc: '管理订阅套餐配置',
      tabPlanConfig: '套餐配置',
      tabUserSubs: '用户订阅',
      selectGroup: '请选择分组',
      groupRequired: '请选择订阅分组',
      priceRequired: '价格必须大于 0',
      validityRequired: '有效期必须大于 0',
      groupMissing: '缺失',
      groupInfo: '分组信息',
      platform: '平台',
      rateMultiplierLabel: '倍率',
      dailyLimit: '日限额',
      weeklyLimit: '周限额',
      monthlyLimit: '月限额',
      unlimited: '无限制',
      searchUserSubs: '搜索用户订阅...',
      daily: '日',
      weekly: '周',
      monthly: '月',
      subsStatus: {
        active: '生效中',
        expired: '已过期',
        revoked: '已撤销',
      },
    },
  },

}
