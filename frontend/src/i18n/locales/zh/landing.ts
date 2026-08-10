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
    heroSubtitle: '一个密钥，畅用多个 AI 模型',
    heroDescription: '无需管理多个订阅账号，一站式接入 Claude、GPT、Gemini 等主流 AI 服务',
    tags: {
      subscriptionToApi: '订阅转 API',
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
    providers: {
      title: '已支持的 AI 模型',
      description: '一个 API，多种选择',
      supported: '已支持',
      soon: '即将推出',
      claude: 'Claude',
      gemini: 'Gemini',
      antigravity: 'Antigravity',
      more: '更多'
    },
    // CTA 区块
    cta: {
      title: '准备好开始了吗？',
      description: '注册即可获得免费试用额度，体验一站式 AI 服务',
      button: '免费注册'
    },
    footer: {
      allRightsReserved: '保留所有权利。'
    },
    styles: {
      platforms: {
        claude: { name: 'Claude' },
        gpt: { name: 'GPT' },
        gemini: { name: 'Gemini' },
        antigravity: { name: 'Antigravity' }
      },
      editorial: {
        eyebrow: '模型路由，从此简单',
        titleLineOne: '一个接口，',
        titleLineTwo: '调用所有好模型。',
        viewExample: '查看调用示例',
        trust: {
          unifiedAuth: '统一鉴权',
          sdkCompatible: '兼容常用 SDK',
          usageBilling: '按实际用量结算'
        },
        terminalLabel: 'API 请求示例',
        request: '请求',
        command: 'curl -X POST /v1/chat/completions',
        routeComment: '正在选择可用路由',
        yourApp: '你的应用',
        gateway: '网关',
        model: '模型',
        responseCode: '200 成功',
        responseBody: 'content: 已就绪',
        details: {
          model: '模型',
          modelValue: '按请求选择',
          route: '路由',
          routeValue: '由网关处理',
          interface: '接口',
          interfaceValue: '兼容 API'
        },
        capabilitiesLabel: '平台能力',
        capabilities: {
          sdk: {
            title: '接入不重写',
            body: '保留现有 SDK 和调用习惯，只需替换 Base URL。'
          },
          routing: {
            title: '统一路由',
            body: '通过同一网关访问已配置的模型路由。'
          },
          billing: {
            title: '用量清晰',
            body: '请求、Token 与费用记录可供查询。'
          }
        },
        directoryEyebrow: '模型目录',
        directoryTitle: '在同一个入口切换模型',
        directoryDescription: '从日常任务到复杂推理，按场景选择已接入的平台。',
        platformDescriptions: {
          claude: '推理、写作与代码',
          gpt: '通用与多模态任务',
          gemini: '多模态与长上下文',
          antigravity: '灵活的路由工作负载'
        }
      },
      operations: {
        eyebrow: '网关控制平面',
        demoNotice: '路由能力演示，不是实时监控',
        capabilitiesLabel: '路由能力概览',
        workspaceLabel: '路由能力演示区',
        metrics: {
          routing: {
            label: '路由方式',
            value: '统一入口',
            note: '按配置选择上游'
          },
          providers: {
            label: '平台示例',
            value: '4 个',
            note: '静态能力展示'
          },
          protocol: {
            label: '传输协议',
            value: 'HTTPS',
            note: '加密传输能力'
          },
          api: {
            label: 'API 接口',
            value: '/v1',
            note: '兼容调用路径'
          }
        },
        matrixTitle: '路由能力矩阵',
        matrixCaption: '配置示例 / 非实时',
        routable: '可路由',
        routes: {
          claude: { name: 'Claude / 主路由', mode: '策略：优先' },
          gpt: { name: 'GPT / 均衡路由', mode: '策略：均衡' },
          gemini: { name: 'Gemini / 快速路由', mode: '策略：低延迟' },
          antigravity: { name: 'Antigravity / 灵活路由', mode: '策略：按配置' }
        },
        quickAccess: '快捷入口',
        disclaimer: '此界面仅说明可配置的路由与协议能力，不读取实时健康、延迟或可用率数据。',
        footerLabel: '路由能力'
      },
      minimal: {
        eyebrow: '独立模型入口',
        established: '始于 {year}',
        kicker: '用更安静的方式构建 AI 产品。',
        notes: {
          endpoint: {
            title: '一个端点',
            body: '用熟悉的接口访问已支持的模型。'
          },
          choice: {
            title: '自由选择',
            body: '切换服务平台，无需改变产品的调用方式。'
          },
          usage: {
            title: '用量可查',
            body: '清晰查看每次请求的用量与计费。'
          }
        }
      },
      catalog: {
        eyebrow: '模型目录',
        title: '为工作选择合适的模型。',
        staticNote: '以下为静态平台目录，实际可用模型以实例配置为准。',
        familiesTitle: '平台目录',
        providerCount: '4 个平台',
        listed: '目录展示',
        capabilityTags: '能力标签',
        models: {
          claude: { description: '适用于审慎推理、写作、分析与生产代码。' },
          gpt: { description: '适用于通用、多模态产品与智能体任务。' },
          gemini: { description: '适用于多模态任务与广泛的上下文场景。' },
          antigravity: { description: '适用于需要灵活路由的模型工作负载。' }
        },
        tags: {
          reasoning: '推理',
          code: '代码',
          longContext: '长上下文',
          general: '通用',
          tools: '工具',
          vision: '视觉',
          multimodal: '多模态',
          fast: '快速',
          context: '上下文',
          routing: '路由',
          flexible: '灵活',
          api: 'API'
        }
      }
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
    title: 'Sub2API 安装向导',
    description: '配置您的 Sub2API 实例',
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
