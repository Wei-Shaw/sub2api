export default {
  batchImageGuide: {
    title: '图片批量生成',
    description: '一次提交多条提示词，任务完成后可统一下载图片结果'
  },
  // Home Page
  home: {
    viewOnGithub: '在 GitHub 上查看',
    viewDocs: '查看文档',
    docs: '文档',
    switchToLight: '切换到浅色模式',
    switchToDark: '切换到深色模式',
    dashboard: '控制台',
    login: '登录',
    getStarted: '立即开始',
    goToDashboard: '进入控制台',
    // 新增：面向用户的价值主张
    heroSubtitle: '一个密钥，畅用所有主流 AI 模型',
    heroDescription: '汇聚国内主流 AI 模型，一个密钥即可接入 DeepSeek、通义千问、Kimi、智谱清言等所有 AI 服务',
    tags: {
      subscriptionToApi: '多模型聚合',
      stickySession: '会话保持',
      realtimeBilling: '按量计费'
    },
    // 用户痛点区块
    painPoints: {
      title: '你是否也遇到这些问题？',
      items: {
        expensive: {
          title: '订阅费用高',
          desc: '每个 AI 服务都要单独订阅，每月支出越来越多'
        },
        complex: {
          title: '多账号难管理',
          desc: '不同平台的账号、密钥分散各处，管理起来很麻烦'
        },
        unstable: {
          title: '服务不稳定',
          desc: '单一账号容易触发限制，影响正常使用'
        },
        noControl: {
          title: '用量无法控制',
          desc: '不知道钱花在哪了，也无法限制团队成员的使用'
        }
      }
    },
    // 解决方案区块
    solutions: {
      title: '我们帮你解决',
      subtitle: '简单三步，开始省心使用 AI'
    },
    features: {
      title: '核心能力',
      description: '一个平台，汇聚所有主流 AI 模型',
      unifiedGateway: '一键接入',
      unifiedGatewayDesc: '获取一个 API 密钥，即可调用所有已接入的 AI 模型，无需分别申请。',
      multiAccount: '稳定可靠',
      multiAccountDesc: '智能调度多个上游账号，自动切换和负载均衡，告别频繁报错。',
      balanceQuota: '用多少付多少',
      balanceQuotaDesc: '按实际使用量计费，支持设置配额上限，团队用量一目了然。'
    },
    // 优势对比
    comparison: {
      title: '为什么选择我们？',
      headers: {
        feature: '对比项',
        official: '官方订阅',
        us: '本平台'
      },
      items: {
        pricing: {
          feature: '付费方式',
          official: '固定月费，用不完也付',
          us: '按量付费，用多少付多少'
        },
        models: {
          feature: '模型选择',
          official: '单一服务商',
          us: '多模型随意切换'
        },
        management: {
          feature: '账号管理',
          official: '每个服务单独管理',
          us: '统一密钥，一站管理'
        },
        stability: {
          feature: '服务稳定性',
          official: '单账号易触发限制',
          us: '多账号池，自动切换'
        },
        control: {
          feature: '用量控制',
          official: '无法限制',
          us: '可设配额、查明细'
        }
      }
    },
    models: {
      title: '支持的模型家族',
      description: '四大主流国产模型家族一站接入，一个密钥自由切换',
      supported: '已支持',
      note: '模型列表持续更新，具体可用模型与定价以控制台为准',
      families: [
        {
          key: 'deepseek',
          name: 'DeepSeek',
          vendor: '深度求索',
          desc: 'V4 旗舰与 Flash 轻量双线并行，深度推理与通用对话的行业性价比标杆',
          models: ['deepseek-v4-pro', 'deepseek-v4-flash', 'deepseek-chat', 'deepseek-reasoner'],
          tags: ['深度推理', '高性价比']
        },
        {
          key: 'qwen',
          name: '通义千问 Qwen',
          vendor: '阿里云',
          desc: '从 Max 旗舰到 Flash 极速的全尺寸矩阵，多模态与长文本能力全面',
          models: ['qwen3.8-max', 'qwen3.8-flash', 'qwen-plus', 'qwen-turbo'],
          tags: ['全尺寸', '多模态']
        },
        {
          key: 'kimi',
          name: 'Kimi',
          vendor: '月之暗面 Moonshot',
          desc: '长上下文先驱，K2.8 系列 Agent 与工具调用能力出色，适合复杂任务编排',
          models: ['kimi-k2.8-preview', 'kimi-k2', 'moonshot-v1-128k'],
          tags: ['长上下文', 'Agent']
        },
        {
          key: 'glm',
          name: 'GLM 智谱',
          vendor: '智谱 Z.ai',
          desc: 'GLM-5.3 旗舰与 Flash 轻量版，代码生成与逻辑推理均衡的国产开源标杆',
          models: ['glm-5.3', 'glm-5.3-flash', 'glm-4.5-air'],
          tags: ['代码', '推理']
        }
      ]
    },
    quickstart: {
      title: '三分钟完成接入',
      description: 'OpenAI 兼容格式，现有代码只需替换 base_url 与密钥即可无缝迁移',
      stepOneTitle: '获取 API 密钥',
      stepOneDesc: '注册后在控制台「API 密钥」页一键创建',
      stepTwoTitle: '替换接入地址',
      stepTwoDesc: '将客户端 base_url 指向本站，密钥填入 sk- 开头的密钥',
      tabCurl: 'curl',
      tabPython: 'Python',
      tabSdk: 'OpenAI SDK',
      compatibilityNote: '兼容所有支持自定义 OpenAI 接入的工具：ChatGPT-Next-Web、LobeChat、Cursor、Claude Code 等'
    },
    // CTA 区块
    cta: {
      title: '准备好开始了吗？',
      description: '注册即可获得免费试用额度，体验全模型一站式接入',
      button: '免费注册'
    },
    // 技术亮点
    techHighlights: {
      title: '技术优势',
      items: {
        compatible: {
          title: 'OpenAI 兼容',
          desc: '完全兼容 OpenAI API 格式，现有代码无需修改即可接入'
        },
        smartRouting: {
          title: '智能路由',
          desc: '基于负载、延迟、错误率自动选择最优上游账号'
        },
        sessionSticky: {
          title: '会话保持',
          desc: '同一对话自动路由到相同上游，保证上下文连贯性'
        },
        realtime: {
          title: '流式传输',
          desc: '支持 SSE 流式响应，Token 逐字输出，体验流畅'
        },
        quotaControl: {
          title: '配额管控',
          desc: '支持 RPM/TPM 限流与余额配额，精确控制团队用量'
        },
        monitoring: {
          title: '实时监控',
          desc: '请求日志、用量统计、错误追踪，运营数据一目了然'
        }
      }
    },
    // FAQ
    faq: {
      title: '常见问题',
      items: {
        q1: {
          question: '什么是多模型集合平台？',
          answer: '我们将多个主流 AI 模型（DeepSeek、通义千问、Kimi、智谱等）聚合在一个平台，您只需一个 API 密钥即可调用所有模型，无需分别注册各个平台。'
        },
        q2: {
          question: 'API 格式兼容吗？',
          answer: '完全兼容 OpenAI Chat Completions API 格式。您现有的代码只需将 base_url 和 api_key 替换为我们的地址和密钥即可使用，无需修改任何业务逻辑。'
        },
        q3: {
          question: '如何计费？',
          answer: '按实际使用量计费，用多少付多少。支持充值余额和设置配额上限，方便控制团队开支。'
        },
        q4: {
          question: '服务稳定性如何保证？',
          answer: '平台采用多账号池化架构，智能调度引擎自动负载均衡和故障切换，单一账号异常不影响整体服务。'
        }
      }
    },
    footer: {
      allRightsReserved: '保留所有权利。'
    }
  },

  // Key Usage Query Page
  keyUsage: {
    title: 'API Key 用量查询',
    subtitle: '输入您的 API Key 以查看实时消费金额与使用状态',
    placeholder: 'sk-ant-mirror-xxxxxxxxxxxx',
    query: '查询',
    querying: '查询中...',
    privacyNote: '您的 Key 仅在浏览器本地处理，不会被存储',
    dateRange: '统计范围:',
    dateRangeToday: '今日',
    dateRange7d: '7 天',
    dateRange30d: '30 天',
    dateRange90d: '90 天',
    dateRangeCustom: '自定义',
    apply: '应用',
    used: '已使用',
    detailInfo: '详细信息',
    tokenStats: 'Token 统计',
    dailyDetail: '按日明细',
    modelStats: '模型用量统计',
    // Table headers
    date: '日期',
    model: '模型',
    requests: '请求数',
    inputTokens: '输入 Tokens',
    outputTokens: '输出 Tokens',
    cacheCreationTokens: '缓存创建',
    cacheReadTokens: '缓存读取',
    cacheWriteTokens: '缓存写入',
    totalTokens: '总 Tokens',
    cost: '费用',
    // Status
    quotaMode: 'Key 限额模式',
    walletBalance: '钱包余额',
    // Ring card titles
    totalQuota: '总额度',
    limit5h: '5 小时限额',
    limitDaily: '日限额',
    limit7d: '7 天限额',
    limitWeekly: '周限额',
    limitMonthly: '月限额',
    // Detail rows
    remainingQuota: '剩余额度',
    expiresAt: '过期时间',
    todayExpires: '(今日到期)',
    daysLeft: '({days} 天)',
    usedQuota: '已用额度',
    resetNow: '即将重置',
    subscriptionType: '订阅类型',
    subscriptionExpires: '订阅到期',
    // Usage stat cells
    todayRequests: '今日请求',
    todayInputTokens: '今日输入',
    todayOutputTokens: '今日输出',
    todayTokens: '今日 Tokens',
    todayCacheCreation: '今日缓存创建',
    todayCacheRead: '今日缓存读取',
    todayCost: '今日费用',
    rpmTpm: 'RPM / TPM',
    totalRequests: '累计请求',
    totalInputTokens: '累计输入',
    totalOutputTokens: '累计输出',
    totalTokensLabel: '累计 Tokens',
    totalCacheCreation: '累计缓存创建',
    totalCacheRead: '累计缓存读取',
    totalCost: '累计费用',
    avgDuration: '平均耗时',
    // Messages
    enterApiKey: '请输入 API Key',
    querySuccess: '查询成功',
    queryFailed: '查询失败',
    queryFailedRetry: '查询失败，请稍后重试',
    noDailyUsage: '暂无按日用量数据',
  },

  // Setup Wizard
  setup: {
    title: '稳得AI 安装向导',
    description: '配置您的 稳得AI 实例',
    database: {
      title: '数据库配置',
      description: '连接到您的 PostgreSQL 数据库',
      host: '主机',
      port: '端口',
      username: '用户名',
      password: '密码',
      databaseName: '数据库名称',
      sslMode: 'SSL 模式',
      passwordPlaceholder: '密码',
      ssl: {
        disable: '禁用',
        require: '要求',
        verifyCa: '验证 CA',
        verifyFull: '完全验证'
      }
    },
    redis: {
      title: 'Redis 配置',
      description: '连接到您的 Redis 服务器',
      host: '主机',
      port: '端口',
      username: '用户名（可选）',
      password: '密码（可选）',
      database: '数据库',
      usernamePlaceholder: '默认用户留空',
      passwordPlaceholder: '密码',
      enableTls: '启用 TLS',
      enableTlsHint: '连接 Redis 时使用 TLS（公共 CA 证书）'
    },
    admin: {
      title: '管理员账户',
      description: '创建您的管理员账户',
      email: '邮箱',
      password: '密码',
      confirmPassword: '确认密码',
      passwordPlaceholder: '至少 8 个字符',
      confirmPasswordPlaceholder: '确认密码',
      passwordMismatch: '密码不匹配'
    },
    ready: {
      title: '准备安装',
      description: '检查您的配置并完成安装',
      database: '数据库',
      redis: 'Redis',
      adminEmail: '管理员邮箱'
    },
    status: {
      testing: '测试中...',
      success: '连接成功',
      testConnection: '测试连接',
      installing: '安装中...',
      completeInstallation: '完成安装',
      completed: '安装完成！',
      redirecting: '正在跳转到登录页面...',
      restarting: '服务正在重启，请稍候...',
      timeout: '服务重启时间超出预期，请手动刷新页面。'
    }
  },

  // Common
}
