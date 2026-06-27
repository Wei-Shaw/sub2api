type LocalePatch = Record<string, unknown>

function isLocalePatch(value: unknown): value is LocalePatch {
  return value !== null && typeof value === 'object' && !Array.isArray(value)
}

export function mergeLocalePatch<T extends LocalePatch>(base: T, patch: LocalePatch): T {
  const target = base as LocalePatch
  for (const [key, value] of Object.entries(patch)) {
    const current = target[key]
    if (isLocalePatch(current) && isLocalePatch(value)) {
      mergeLocalePatch(current, value)
      continue
    }
    target[key] = value
  }
  return base
}

export function cloneLocalePatch<T extends LocalePatch>(value: T): T {
  return JSON.parse(JSON.stringify(value)) as T
}

const emailTemplateMeta = {
  en: {
    messageKinds: { optional: 'Optional', transactional: 'Transactional' },
    categories: { notification: 'Notification', auth: 'Auth', subscription: 'Subscription', billing: 'Billing', admin: 'Admin', riskControl: 'Risk Control', ops: 'Ops' },
    events: {
      authVerifyCode: { label: 'Email Verification Code', timing: 'Sent for registration, email binding, OAuth pending email completion, or TOTP email verification.' },
      authPasswordReset: { label: 'Password Reset', timing: 'Sent when a user requests a password reset link.' },
      notificationEmailVerifyCode: { label: 'Notification Email Verification', timing: 'Sent when a user adds and verifies an extra notification email address.' },
      subscriptionPurchaseSuccess: { label: 'Subscription Activated', timing: 'Sent after a subscription order is paid and the subscription is activated or extended.' },
      subscriptionExpiryReminder: { label: 'Subscription Expiry Reminder', timing: 'Sent by the background job when an active subscription has 7, 3, or 1 day remaining. It can be disabled in Email settings.' },
      balanceLow: { label: 'Low Balance Alert', timing: "Sent when a user's balance drops below the global or personal reminder threshold." },
      balanceRechargeSuccess: { label: 'Balance Recharge Success', timing: 'Sent after a balance recharge order is paid and credited.' },
      accountQuotaAlert: { label: 'Account Quota Alert', timing: 'Sent to admin notification emails when an upstream account reaches the configured quota alert threshold.' },
      contentModerationViolationNotice: { label: 'Risk Control Violation Notice', timing: 'Sent when a user request triggers content moderation or risk-control rules but the account is not disabled yet.' },
      contentModerationAccountDisabled: { label: 'Risk Control Account Disabled', timing: 'Sent when content moderation reaches the ban threshold and automatically disables the user account.' },
      opsAlert: { label: 'Ops Alert', timing: 'Sent to ops recipients when an ops monitoring rule fires and email notification settings allow it.' },
      opsScheduledReport: { label: 'Ops Scheduled Report', timing: 'Sent when a configured daily, weekly, error digest, or account health report reaches its scheduled send time.' }
    }
  },
  zh: {
    messageKinds: { optional: '可退订通知', transactional: '事务邮件' },
    categories: { notification: '通知', auth: '认证安全', subscription: '订阅', billing: '计费', admin: '管理告警', riskControl: '风控', ops: '运维' },
    events: {
      authVerifyCode: { label: '邮箱验证码', timing: '注册、绑定邮箱、OAuth 补全邮箱或 TOTP 邮箱校验时发送。' },
      authPasswordReset: { label: '密码重置', timing: '用户请求密码重置链接时发送。' },
      notificationEmailVerifyCode: { label: '通知邮箱验证码', timing: '用户添加并验证额外通知邮箱时发送。' },
      subscriptionPurchaseSuccess: { label: '订阅开通成功', timing: '订阅订单完成支付并成功开通或续期后发送。' },
      subscriptionExpiryReminder: { label: '订阅到期提醒', timing: '后台任务在订阅仍有效且距离到期剩余 7 天、3 天、1 天时各发送一次，可通过邮件设置中的开关关闭。' },
      balanceLow: { label: '余额不足提醒', timing: '用户余额低于全局或个人配置的提醒阈值时发送。' },
      balanceRechargeSuccess: { label: '余额充值成功', timing: '余额充值订单支付完成并入账后发送。' },
      accountQuotaAlert: { label: '账号限额告警', timing: '上游账号的用量达到配置的额度告警阈值时发送给管理员通知邮箱。' },
      contentModerationViolationNotice: { label: '内容审计违规提醒', timing: '用户请求命中内容审计或风控规则、但尚未被禁用时发送。' },
      contentModerationAccountDisabled: { label: '内容审计禁用账号', timing: '内容审计违规次数达到封禁阈值并自动禁用用户账号时发送。' },
      opsAlert: { label: '运维告警', timing: '运维监控规则触发告警并满足邮件通知配置时发送给运维收件人。' },
      opsScheduledReport: { label: '运维定时报表', timing: '运维日报、周报、错误摘要或账号健康报表到达配置的发送时间时发送。' }
    }
  },
  vi: {
    messageKinds: { optional: 'Thông báo có thể hủy đăng ký', transactional: 'Email giao dịch' },
    categories: { notification: 'Thông báo', auth: 'Bảo mật xác thực', subscription: 'Đăng ký', billing: 'Thanh toán', admin: 'Cảnh báo quản trị', riskControl: 'Kiểm soát rủi ro', ops: 'Vận hành' },
    events: {
      authVerifyCode: { label: 'Mã xác minh email', timing: 'Gửi khi đăng ký, liên kết email, hoàn tất email OAuth hoặc xác minh email TOTP.' },
      authPasswordReset: { label: 'Đặt lại mật khẩu', timing: 'Gửi khi người dùng yêu cầu liên kết đặt lại mật khẩu.' },
      notificationEmailVerifyCode: { label: 'Mã xác minh email thông báo', timing: 'Gửi khi người dùng thêm và xác minh email nhận thông báo bổ sung.' },
      subscriptionPurchaseSuccess: { label: 'Đăng ký đã kích hoạt', timing: 'Gửi sau khi đơn đăng ký thanh toán xong và đăng ký được kích hoạt hoặc gia hạn.' },
      subscriptionExpiryReminder: { label: 'Nhắc đăng ký sắp hết hạn', timing: 'Job nền gửi khi đăng ký còn hiệu lực và còn 7, 3 hoặc 1 ngày; có thể tắt trong cài đặt Email.' },
      balanceLow: { label: 'Cảnh báo số dư thấp', timing: 'Gửi khi số dư người dùng thấp hơn ngưỡng nhắc toàn cục hoặc cá nhân.' },
      balanceRechargeSuccess: { label: 'Nạp số dư thành công', timing: 'Gửi sau khi đơn nạp số dư thanh toán xong và được ghi có.' },
      accountQuotaAlert: { label: 'Cảnh báo hạn mức tài khoản', timing: 'Gửi tới email thông báo quản trị khi tài khoản upstream đạt ngưỡng cảnh báo hạn mức đã cấu hình.' },
      contentModerationViolationNotice: { label: 'Nhắc vi phạm kiểm soát rủi ro', timing: 'Gửi khi yêu cầu người dùng chạm quy tắc kiểm duyệt nội dung hoặc kiểm soát rủi ro nhưng tài khoản chưa bị vô hiệu hóa.' },
      contentModerationAccountDisabled: { label: 'Vô hiệu hóa tài khoản do kiểm soát rủi ro', timing: 'Gửi khi số lần vi phạm kiểm duyệt nội dung đạt ngưỡng khóa và tự động vô hiệu hóa tài khoản.' },
      opsAlert: { label: 'Cảnh báo vận hành', timing: 'Gửi tới người nhận vận hành khi quy tắc giám sát vận hành kích hoạt và cài đặt email cho phép.' },
      opsScheduledReport: { label: 'Báo cáo vận hành định kỳ', timing: 'Gửi khi báo cáo ngày, tuần, tóm tắt lỗi hoặc sức khỏe tài khoản đến lịch gửi đã cấu hình.' }
    }
  },
  ko: {
    messageKinds: { optional: '수신 거부 가능 알림', transactional: '트랜잭션 메일' },
    categories: { notification: '알림', auth: '인증 보안', subscription: '구독', billing: '결제', admin: '관리자 알림', riskControl: '위험 제어', ops: '운영' },
    events: {
      authVerifyCode: { label: '이메일 인증 코드', timing: '회원가입, 이메일 연결, OAuth 보류 이메일 완료 또는 TOTP 이메일 인증 시 발송됩니다.' },
      authPasswordReset: { label: '비밀번호 재설정', timing: '사용자가 비밀번호 재설정 링크를 요청할 때 발송됩니다.' },
      notificationEmailVerifyCode: { label: '알림 이메일 인증', timing: '사용자가 추가 알림 이메일 주소를 등록하고 인증할 때 발송됩니다.' },
      subscriptionPurchaseSuccess: { label: '구독 활성화', timing: '구독 주문 결제가 완료되고 구독이 활성화되거나 연장된 뒤 발송됩니다.' },
      subscriptionExpiryReminder: { label: '구독 만료 알림', timing: '활성 구독이 7일, 3일 또는 1일 남았을 때 백그라운드 작업이 발송합니다. 이메일 설정에서 비활성화할 수 있습니다.' },
      balanceLow: { label: '잔액 부족 알림', timing: '사용자 잔액이 전역 또는 개인 알림 임계값보다 낮아질 때 발송됩니다.' },
      balanceRechargeSuccess: { label: '잔액 충전 성공', timing: '잔액 충전 주문 결제가 완료되고 금액이 반영된 뒤 발송됩니다.' },
      accountQuotaAlert: { label: '계정 한도 알림', timing: '업스트림 계정이 설정된 한도 알림 임계값에 도달하면 관리자 알림 이메일로 발송됩니다.' },
      contentModerationViolationNotice: { label: '위험 제어 위반 알림', timing: '사용자 요청이 콘텐츠 감사 또는 위험 제어 규칙에 걸렸지만 계정이 아직 비활성화되지 않았을 때 발송됩니다.' },
      contentModerationAccountDisabled: { label: '위험 제어 계정 비활성화', timing: '콘텐츠 감사 위반 횟수가 차단 임계값에 도달해 계정이 자동 비활성화될 때 발송됩니다.' },
      opsAlert: { label: '운영 알림', timing: '운영 모니터링 규칙이 트리거되고 이메일 알림 설정이 허용할 때 운영 수신자에게 발송됩니다.' },
      opsScheduledReport: { label: '운영 정기 보고서', timing: '설정된 일간, 주간, 오류 요약 또는 계정 상태 보고서가 예약된 발송 시간에 도달하면 발송됩니다.' }
    }
  }
} as const

const adminLocalePatchBase = {
  en: {
    admin: {
      affiliates: {
        records: {
          payAmount: 'Pay Amount'
        }
      },
      channelMonitor: {
        form: {
          apiMode: 'API Mode',
          apiModeChatCompletions: 'Chat Completions',
          apiModeChatCompletionsHint: 'Send OpenAI-compatible /chat/completions requests to this monitor.',
          apiModeResponses: 'Responses',
          apiModeResponsesHint: 'Send OpenAI-compatible /responses requests to this monitor.'
        }
      },
      channels: {
        emptyModelsInPricing: 'No models are configured in pricing',
        noGroupsSelected: 'No groups selected',
        form: {
          syncLatestModels: 'Sync latest models',
          syncingModels: 'Syncing models...',
          syncModelsSuccess: 'Model list synced',
          syncModelsError: 'Failed to sync model list',
          syncModelsAlreadyUpToDate: 'Model list is already up to date'
        }
      },
      ops: {
        autoRefreshRemaining: 'Refresh in {seconds}s',
        runtime: {
          metricThresholds: 'Metric thresholds',
          metricThresholdsHint: 'Tune the warning thresholds used by the runtime dashboard and alerts.',
          requestErrorRateMaxPercent: 'Request error rate max (%)',
          requestErrorRateMaxPercentHint: 'Alert when client request failures exceed this percentage.',
          upstreamErrorRateMaxPercent: 'Upstream error rate max (%)',
          upstreamErrorRateMaxPercentHint: 'Alert when upstream/provider failures exceed this percentage.',
          ttftP99MaxMs: 'TTFT P99 max (ms)',
          ttftP99MaxMsHint: 'Alert when p99 time-to-first-token is higher than this value.',
          slaMinPercent: 'Minimum SLA (%)',
          slaMinPercentHint: 'Alert when availability falls below this percentage.'
        }
      },
      redeem: {
        batchUpdate: 'Batch Update',
        batchUpdateTitle: 'Batch update redeem codes',
        batchUpdateSuccess: 'Updated {count} redeem codes',
        batchFields: {
          status: 'Status',
          group: 'Group',
          expiresAt: 'Expiration',
          notes: 'Notes'
        },
        batchNotesPlaceholder: 'Optional notes for selected codes',
        clearGroup: 'Clear group',
        clearSelection: 'Clear selection',
        codeExpiry: 'Code expiration',
        customExpiry: 'Custom expiration',
        customExpiryDays: 'Custom days',
        expiryPresetDays: '{days} days',
        neverExpires: 'Never expires',
        noBatchFieldsSelected: 'Select at least one field to update',
        selectCodesFirst: 'Select redeem codes first',
        selectedCount: '{count} selected',
        expiryDaysRequired: 'Enter a valid number of days',
        failedToBatchUpdate: 'Failed to batch update redeem codes',
        usagePolicy: 'Usage policy',
        usagePolicyHint: 'Single-use consumes one code globally. Once-per-user can share a campaign scope across users.',
        usageScope: 'Campaign scope',
        usageScopePlaceholder: 'Optional shared campaign key',
        maxTotalUses: 'Max total uses',
        maxTotalUsesHint: '0 means unlimited total uses',
        maxTotalUsesInvalid: 'Max total uses must be 0 or greater',
        maxUsesPerUser: 'Max uses per user',
        usagePolicies: {
          single_use: 'Single use',
          once_per_user: 'Once per user'
        },
        columns: {
          expiresAt: 'Expires At',
          usagePolicy: 'Policy',
          usedCount: 'Used'
        }
      },
      riskControl: {
        action: {
          keywordBlock: 'Keyword block'
        },
        keywordBlockingMode: 'Keyword blocking mode',
        keywordModeKeywordOnly: 'Keyword only',
        keywordModeKeywordOnlyDesc: 'Block requests only when configured keywords are matched.',
        keywordModeKeywordOnlyNotice: 'API moderation is skipped in keyword-only mode.',
        keywordModeApiOnly: 'API moderation only',
        keywordModeApiOnlyDesc: 'Use the moderation API without keyword pre-blocking.',
        keywordModeApiOnlyNotice: 'Keyword rules are not applied in API-only mode.',
        keywordModeKeywordAndApi: 'Keywords + API moderation',
        keywordModeKeywordAndApiDesc: 'Apply keyword pre-blocking first, then use the moderation API for remaining requests.',
        blockedKeywords: 'Blocked keywords',
        blockedKeywordCount: '{count} blocked keywords',
        blockedKeywordsDescription: 'Requests containing these keywords are blocked before they reach the upstream provider.',
        blockedKeywordsLimit: 'Up to {max} keywords, one per line',
        blockedKeywordsModeWarning: 'Keyword blocking is active in {mode} mode',
        blockedKeywordsPlaceholder: 'One keyword or phrase per line',
        blockedKeywordsPreBlockHint: 'Keyword hits are rejected immediately in the pre-block path.',
        defaultBlockMessage: 'Your request was blocked by risk-control rules.'
      },
      settings: {
        authSourceDefaults: {
          sources: {
            google: {
              title: 'Google signup',
              description: 'Default quota grants for Google OAuth signups.'
            },
            github: {
              title: 'GitHub signup',
              description: 'Default quota grants for GitHub OAuth signups.'
            }
          }
        },
        emailOAuthSettings: {
          title: 'Email OAuth settings',
          description: 'Configure Google and GitHub OAuth login for email-based accounts.',
          googleSetupGuide: 'Create an OAuth client in Google Cloud Console and add the callback URL below.',
          googleHint: 'Used for Google OAuth login and account binding.',
          githubSetupPrefix: 'Create a GitHub OAuth App and set the callback URL to ',
          githubSetupSuffix: '.',
          githubHint: 'Used for GitHub OAuth login and account binding.',
          secretConfiguredPlaceholder: 'Secret configured. Leave empty to keep it.',
          callbackUrlSetAndCopied: 'Callback URL generated and copied'
        },
        emailTemplates: {
          title: 'Email templates',
          description: 'Edit localized email subjects and HTML templates.',
          event: 'Event',
          locale: 'Locale',
          localeZh: 'Chinese',
          localeEn: 'English',
          subject: 'Subject',
          subjectPlaceholder: 'Email subject',
          html: 'HTML body',
          htmlPlaceholder: 'Email HTML content',
          placeholders: 'Placeholders',
          placeholdersHelp: 'Click a placeholder to copy it into the template.',
          preview: 'Preview',
          previewing: 'Previewing...',
          livePreview: 'Live preview',
          noPreview: 'No preview available',
          previewSecurityHint: 'Preview is sanitized before rendering.',
          customized: 'Customized',
          empty: 'Select an event and locale to edit a template.',
          save: 'Save template',
          saving: 'Saving...',
          saveSuccess: 'Template saved',
          restoreOfficial: 'Restore official template',
          restoring: 'Restoring...',
          restoreConfirm: 'Restore the official template? Your custom changes will be replaced.',
          restoreSuccess: 'Official template restored',
          placeholderCopied: 'Placeholder copied',
          validationRequired: 'Subject and HTML body are required',
          ...emailTemplateMeta.en
        },
        wechatConnect: {
          browserRedirectUrlLabel: 'Browser redirect URL',
          browserRedirectUrlHint: 'Used after OAuth finishes in a normal browser.',
          unionIdWarning: 'UnionID may be unavailable unless the account is bound to the same WeChat Open Platform account.',
          mpMobileConflict: 'Official Account and mobile app modes should not share the same AppID/AppSecret unless they belong to the same WeChat application.',
          modes: {
            open: {
              title: 'Open Platform',
              description: 'Use QR-code authorization outside WeChat.',
              appIdLabel: 'Open Platform App ID',
              appIdPlaceholder: 'WeChat Open Platform App ID',
              appSecretLabel: 'Open Platform App Secret',
              appSecretPlaceholder: 'WeChat Open Platform App Secret'
            },
            mp: {
              title: 'Official Account',
              description: 'Use Official Account authorization inside WeChat.',
              appIdLabel: 'Official Account App ID',
              appIdPlaceholder: 'WeChat Official Account App ID',
              appSecretLabel: 'Official Account App Secret',
              appSecretPlaceholder: 'WeChat Official Account App Secret'
            },
            mobile: {
              title: 'Mobile App',
              description: 'Use WeChat mobile-app OAuth for desktop/mobile app login.',
              appIdLabel: 'Mobile App ID',
              appIdPlaceholder: 'WeChat Mobile App ID',
              appSecretLabel: 'Mobile App Secret',
              appSecretPlaceholder: 'WeChat Mobile App Secret'
            }
          }
        }
      },
      subscriptions: {
        searchDeviceCode: 'Search by device code...',
        quotaEndsInMinutes: 'Ends in {minutes}m',
        quotaEndsInHoursMinutes: 'Ends in {hours}h {minutes}m',
        quotaEndsInDaysHours: 'Ends in {days}d {hours}h'
      },
      users: {
        sortBy: 'Sort by',
        sortCurrentPageOnly: 'Sort applies to the current page only',
        columnAlwaysVisible: 'Always visible',
        passwordCopied: 'Password copied',
        columns: {
          usageOpenAI: 'OpenAI usage',
          usageAnthropic: 'Anthropic usage',
          usageGemini: 'Gemini usage',
          usageAntigravity: 'Antigravity usage'
        }
      }
    },
    payment: {
      admin: {
        colDeviceCode: 'Device Code'
      }
    }
  },
  zh: {
    admin: {
      affiliates: { records: { payAmount: '支付金额' } },
      channelMonitor: {
        form: {
          apiMode: 'API 模式',
          apiModeChatCompletions: 'Chat Completions',
          apiModeChatCompletionsHint: '向该监控发送 OpenAI 兼容的 /chat/completions 请求。',
          apiModeResponses: 'Responses',
          apiModeResponsesHint: '向该监控发送 OpenAI 兼容的 /responses 请求。'
        }
      },
      channels: {
        emptyModelsInPricing: '价格配置中暂无模型',
        noGroupsSelected: '未选择分组',
        form: {
          syncLatestModels: '同步最新模型',
          syncingModels: '正在同步模型...',
          syncModelsSuccess: '模型列表已同步',
          syncModelsError: '同步模型列表失败',
          syncModelsAlreadyUpToDate: '模型列表已是最新'
        }
      },
      ops: {
        autoRefreshRemaining: '{seconds} 秒后刷新',
        runtime: {
          metricThresholds: '指标阈值',
          metricThresholdsHint: '调整运行时看板和告警使用的阈值。',
          requestErrorRateMaxPercent: '请求错误率上限（%）',
          requestErrorRateMaxPercentHint: '当客户端请求失败率超过该百分比时告警。',
          upstreamErrorRateMaxPercent: '上游错误率上限（%）',
          upstreamErrorRateMaxPercentHint: '当上游/供应商失败率超过该百分比时告警。',
          ttftP99MaxMs: 'TTFT P99 上限（ms）',
          ttftP99MaxMsHint: '当首 token P99 延迟高于该值时告警。',
          slaMinPercent: '最低 SLA（%）',
          slaMinPercentHint: '当可用性低于该百分比时告警。'
        }
      },
      redeem: {
        batchUpdate: '批量更新',
        batchUpdateTitle: '批量更新兑换码',
        batchUpdateSuccess: '已更新 {count} 个兑换码',
        batchFields: { status: '状态', group: '分组', expiresAt: '过期时间', notes: '备注' },
        batchNotesPlaceholder: '选中兑换码的可选备注',
        clearGroup: '清除分组',
        clearSelection: '清除选择',
        codeExpiry: '兑换码过期时间',
        customExpiry: '自定义过期时间',
        customExpiryDays: '自定义天数',
        expiryPresetDays: '{days} 天',
        neverExpires: '永不过期',
        noBatchFieldsSelected: '请至少选择一个要更新的字段',
        selectCodesFirst: '请先选择兑换码',
        selectedCount: '已选择 {count} 个',
        expiryDaysRequired: '请输入有效天数',
        failedToBatchUpdate: '批量更新兑换码失败',
        usagePolicy: '使用策略',
        usagePolicyHint: '单次使用表示一个码全局只能使用一次；每用户一次可用同一活动范围跨用户复用。',
        usageScope: '活动范围',
        usageScopePlaceholder: '可选的共享活动标识',
        maxTotalUses: '总使用上限',
        maxTotalUsesHint: '0 表示不限总次数',
        maxTotalUsesInvalid: '总使用上限必须大于等于 0',
        maxUsesPerUser: '每用户上限',
        usagePolicies: { single_use: '单次使用', once_per_user: '每用户一次' },
        columns: { expiresAt: '过期时间', usagePolicy: '策略', usedCount: '已用' }
      },
      riskControl: {
        action: { keywordBlock: '关键词拦截' },
        keywordBlockingMode: '关键词拦截模式',
        keywordModeKeywordOnly: '仅关键词',
        keywordModeKeywordOnlyDesc: '仅在命中配置关键词时拦截请求。',
        keywordModeKeywordOnlyNotice: '仅关键词模式下不会调用审核 API。',
        keywordModeApiOnly: '仅 API 审核',
        keywordModeApiOnlyDesc: '使用审核 API，不进行关键词前置拦截。',
        keywordModeApiOnlyNotice: '仅 API 模式下不会应用关键词规则。',
        keywordModeKeywordAndApi: '关键词 + API 审核',
        keywordModeKeywordAndApiDesc: '先执行关键词前置拦截，再对剩余请求调用审核 API。',
        blockedKeywords: '拦截关键词',
        blockedKeywordCount: '{count} 个拦截关键词',
        blockedKeywordsDescription: '包含这些关键词的请求会在到达上游前被拦截。',
        blockedKeywordsLimit: '最多 {max} 个关键词，每行一个',
        blockedKeywordsModeWarning: '关键词拦截已在 {mode} 模式启用',
        blockedKeywordsPlaceholder: '每行一个关键词或短语',
        blockedKeywordsPreBlockHint: '关键词命中会在前置拦截路径中立即拒绝。',
        defaultBlockMessage: '你的请求已被风控规则拦截。'
      },
      settings: {
        authSourceDefaults: { sources: { google: { title: 'Google 注册', description: 'Google OAuth 注册用户的默认配额。' }, github: { title: 'GitHub 注册', description: 'GitHub OAuth 注册用户的默认配额。' } } },
        emailOAuthSettings: {
          title: '邮箱 OAuth 设置',
          description: '配置邮箱账号的 Google 和 GitHub OAuth 登录。',
          googleSetupGuide: '在 Google Cloud Console 创建 OAuth 客户端，并添加下面的回调地址。',
          googleHint: '用于 Google OAuth 登录和账号绑定。',
          githubSetupPrefix: '创建 GitHub OAuth App，并将回调地址设置为 ',
          githubSetupSuffix: '。',
          githubHint: '用于 GitHub OAuth 登录和账号绑定。',
          secretConfiguredPlaceholder: '密钥已配置，留空保持当前值。',
          callbackUrlSetAndCopied: '回调地址已生成并复制'
        },
        emailTemplates: {
          title: '邮件模板', description: '编辑多语言邮件标题和 HTML 模板。', event: '事件', locale: '语言', localeZh: '中文', localeEn: '英文', subject: '标题', subjectPlaceholder: '邮件标题', html: 'HTML 正文', htmlPlaceholder: '邮件 HTML 内容', placeholders: '占位符', placeholdersHelp: '点击占位符可复制到模板。', preview: '预览', previewing: '预览中...', livePreview: '实时预览', noPreview: '暂无预览', previewSecurityHint: '预览内容会先清洗再渲染。', customized: '已自定义', empty: '请选择事件和语言后编辑模板。', save: '保存模板', saving: '保存中...', saveSuccess: '模板已保存', restoreOfficial: '恢复官方模板', restoring: '恢复中...', restoreConfirm: '恢复官方模板？自定义内容会被替换。', restoreSuccess: '官方模板已恢复', placeholderCopied: '占位符已复制', validationRequired: '标题和 HTML 正文为必填项', ...emailTemplateMeta.zh
        },
        wechatConnect: {
          browserRedirectUrlLabel: '浏览器跳转地址', browserRedirectUrlHint: '普通浏览器 OAuth 完成后跳转到该地址。', unionIdWarning: '除非绑定到同一个微信开放平台账号，否则可能无法获取 UnionID。', mpMobileConflict: '公众号和移动应用模式不应共用同一个 AppID/AppSecret，除非它们属于同一个微信应用。',
          modes: {
            open: { title: '开放平台', description: '在微信外使用扫码授权。', appIdLabel: '开放平台 AppID', appIdPlaceholder: '微信开放平台 AppID', appSecretLabel: '开放平台 AppSecret', appSecretPlaceholder: '微信开放平台 AppSecret' },
            mp: { title: '公众号', description: '在微信内使用公众号授权。', appIdLabel: '公众号 AppID', appIdPlaceholder: '微信公众号 AppID', appSecretLabel: '公众号 AppSecret', appSecretPlaceholder: '微信公众号 AppSecret' },
            mobile: { title: '移动应用', description: '使用微信移动应用 OAuth 进行桌面/移动端登录。', appIdLabel: '移动应用 AppID', appIdPlaceholder: '微信移动应用 AppID', appSecretLabel: '移动应用 AppSecret', appSecretPlaceholder: '微信移动应用 AppSecret' }
          }
        }
      },
      subscriptions: { searchDeviceCode: '按设备码搜索...', quotaEndsInMinutes: '{minutes} 分钟后结束', quotaEndsInHoursMinutes: '{hours} 小时 {minutes} 分钟后结束', quotaEndsInDaysHours: '{days} 天 {hours} 小时后结束' },
      users: { sortBy: '排序', sortCurrentPageOnly: '排序仅作用于当前页', columnAlwaysVisible: '始终显示', passwordCopied: '密码已复制', columns: { usageOpenAI: 'OpenAI 用量', usageAnthropic: 'Anthropic 用量', usageGemini: 'Gemini 用量', usageAntigravity: 'Antigravity 用量' } }
    },
    payment: { admin: { colDeviceCode: '设备码' } }
  },
  vi: {
    admin: {
      affiliates: { records: { payAmount: 'Số tiền thanh toán' } },
      channelMonitor: { form: { apiMode: 'Chế độ API', apiModeChatCompletions: 'Chat Completions', apiModeChatCompletionsHint: 'Gửi request tương thích OpenAI /chat/completions tới monitor này.', apiModeResponses: 'Responses', apiModeResponsesHint: 'Gửi request tương thích OpenAI /responses tới monitor này.' } },
      channels: { emptyModelsInPricing: 'Chưa cấu hình mô hình trong bảng giá', noGroupsSelected: 'Chưa chọn nhóm nào', form: { syncLatestModels: 'Đồng bộ mô hình mới nhất', syncingModels: 'Đang đồng bộ mô hình...', syncModelsSuccess: 'Đã đồng bộ danh sách mô hình', syncModelsError: 'Đồng bộ danh sách mô hình thất bại', syncModelsAlreadyUpToDate: 'Danh sách mô hình đã là mới nhất' } },
      ops: { autoRefreshRemaining: 'Làm mới sau {seconds}s', runtime: { metricThresholds: 'Ngưỡng chỉ số', metricThresholdsHint: 'Điều chỉnh ngưỡng cảnh báo cho dashboard runtime và alert.', requestErrorRateMaxPercent: 'Tỷ lệ lỗi request tối đa (%)', requestErrorRateMaxPercentHint: 'Cảnh báo khi tỷ lệ request của client lỗi vượt quá phần trăm này.', upstreamErrorRateMaxPercent: 'Tỷ lệ lỗi upstream tối đa (%)', upstreamErrorRateMaxPercentHint: 'Cảnh báo khi lỗi upstream/provider vượt quá phần trăm này.', ttftP99MaxMs: 'TTFT P99 tối đa (ms)', ttftP99MaxMsHint: 'Cảnh báo khi P99 time-to-first-token cao hơn giá trị này.', slaMinPercent: 'SLA tối thiểu (%)', slaMinPercentHint: 'Cảnh báo khi độ khả dụng thấp hơn phần trăm này.' } },
      redeem: { batchUpdate: 'Cập nhật hàng loạt', batchUpdateTitle: 'Cập nhật hàng loạt mã redeem', batchUpdateSuccess: 'Đã cập nhật {count} mã redeem', batchFields: { status: 'Trạng thái', group: 'Nhóm', expiresAt: 'Hết hạn', notes: 'Ghi chú' }, batchNotesPlaceholder: 'Ghi chú tùy chọn cho các mã đã chọn', clearGroup: 'Xóa nhóm', clearSelection: 'Bỏ chọn', codeExpiry: 'Thời hạn mã', customExpiry: 'Thời hạn tùy chỉnh', customExpiryDays: 'Số ngày tùy chỉnh', expiryPresetDays: '{days} ngày', neverExpires: 'Không bao giờ hết hạn', noBatchFieldsSelected: 'Chọn ít nhất một trường cần cập nhật', selectCodesFirst: 'Hãy chọn mã redeem trước', selectedCount: 'Đã chọn {count}', expiryDaysRequired: 'Nhập số ngày hợp lệ', failedToBatchUpdate: 'Cập nhật hàng loạt mã redeem thất bại', usagePolicy: 'Chính sách sử dụng', usagePolicyHint: 'Một lần nghĩa là mã chỉ dùng một lần toàn hệ thống. Mỗi người một lần có thể dùng chung phạm vi chiến dịch.', usageScope: 'Phạm vi chiến dịch', usageScopePlaceholder: 'Khóa chiến dịch dùng chung (tùy chọn)', maxTotalUses: 'Tổng lượt dùng tối đa', maxTotalUsesHint: '0 nghĩa là không giới hạn tổng lượt', maxTotalUsesInvalid: 'Tổng lượt dùng tối đa phải từ 0 trở lên', maxUsesPerUser: 'Lượt dùng mỗi user', usagePolicies: { single_use: 'Dùng một lần', once_per_user: 'Mỗi user một lần' }, columns: { expiresAt: 'Hết hạn', usagePolicy: 'Chính sách', usedCount: 'Đã dùng' } },
      riskControl: { action: { keywordBlock: 'Chặn từ khóa' }, keywordBlockingMode: 'Chế độ chặn từ khóa', keywordModeKeywordOnly: 'Chỉ từ khóa', keywordModeKeywordOnlyDesc: 'Chỉ chặn request khi khớp các từ khóa đã cấu hình.', keywordModeKeywordOnlyNotice: 'Chế độ chỉ từ khóa sẽ bỏ qua API kiểm duyệt.', keywordModeApiOnly: 'Chỉ API kiểm duyệt', keywordModeApiOnlyDesc: 'Dùng API kiểm duyệt và không chặn trước bằng từ khóa.', keywordModeApiOnlyNotice: 'Chế độ chỉ API sẽ không áp dụng quy tắc từ khóa.', keywordModeKeywordAndApi: 'Từ khóa + API kiểm duyệt', keywordModeKeywordAndApiDesc: 'Chặn trước bằng từ khóa, sau đó dùng API kiểm duyệt cho request còn lại.', blockedKeywords: 'Từ khóa bị chặn', blockedKeywordCount: '{count} từ khóa bị chặn', blockedKeywordsDescription: 'Request chứa các từ khóa này sẽ bị chặn trước khi tới upstream.', blockedKeywordsLimit: 'Tối đa {max} từ khóa, mỗi dòng một từ/cụm', blockedKeywordsModeWarning: 'Chặn từ khóa đang bật trong chế độ {mode}', blockedKeywordsPlaceholder: 'Mỗi dòng một từ khóa hoặc cụm từ', blockedKeywordsPreBlockHint: 'Từ khóa khớp sẽ bị từ chối ngay trong luồng chặn trước.', defaultBlockMessage: 'Request của bạn đã bị chặn bởi quy tắc kiểm soát rủi ro.' },
      settings: { authSourceDefaults: { sources: { google: { title: 'Đăng ký Google', description: 'Quota mặc định cho người dùng đăng ký bằng Google OAuth.' }, github: { title: 'Đăng ký GitHub', description: 'Quota mặc định cho người dùng đăng ký bằng GitHub OAuth.' } } }, emailOAuthSettings: { title: 'Cài đặt Email OAuth', description: 'Cấu hình đăng nhập Google và GitHub OAuth cho tài khoản email.', googleSetupGuide: 'Tạo OAuth client trong Google Cloud Console và thêm Callback URL bên dưới.', googleHint: 'Dùng cho đăng nhập Google OAuth và liên kết tài khoản.', githubSetupPrefix: 'Tạo GitHub OAuth App và đặt Callback URL thành ', githubSetupSuffix: '.', githubHint: 'Dùng cho đăng nhập GitHub OAuth và liên kết tài khoản.', secretConfiguredPlaceholder: 'Đã cấu hình secret. Để trống để giữ nguyên.', callbackUrlSetAndCopied: 'Đã tạo và copy Callback URL' }, emailTemplates: { title: 'Mẫu email', description: 'Chỉnh sửa tiêu đề và HTML email theo từng ngôn ngữ.', event: 'Sự kiện', locale: 'Ngôn ngữ', localeZh: 'Tiếng Trung', localeEn: 'Tiếng Anh', subject: 'Tiêu đề', subjectPlaceholder: 'Tiêu đề email', html: 'Nội dung HTML', htmlPlaceholder: 'Nội dung HTML của email', placeholders: 'Biến placeholder', placeholdersHelp: 'Bấm vào placeholder để copy vào mẫu.', preview: 'Xem trước', previewing: 'Đang xem trước...', livePreview: 'Xem trước trực tiếp', noPreview: 'Chưa có bản xem trước', previewSecurityHint: 'Bản xem trước sẽ được sanitize trước khi hiển thị.', customized: 'Đã tùy chỉnh', empty: 'Chọn sự kiện và ngôn ngữ để chỉnh sửa mẫu.', save: 'Lưu mẫu', saving: 'Đang lưu...', saveSuccess: 'Đã lưu mẫu', restoreOfficial: 'Khôi phục mẫu mặc định', restoring: 'Đang khôi phục...', restoreConfirm: 'Khôi phục mẫu mặc định? Nội dung tùy chỉnh sẽ bị thay thế.', restoreSuccess: 'Đã khôi phục mẫu mặc định', placeholderCopied: 'Đã copy placeholder', validationRequired: 'Tiêu đề và nội dung HTML là bắt buộc', ...emailTemplateMeta.vi }, wechatConnect: { browserRedirectUrlLabel: 'URL chuyển hướng trình duyệt', browserRedirectUrlHint: 'Dùng sau khi OAuth hoàn tất trong trình duyệt thường.', unionIdWarning: 'UnionID có thể không khả dụng nếu tài khoản không cùng WeChat Open Platform.', mpMobileConflict: 'Official Account và Mobile App không nên dùng chung AppID/AppSecret trừ khi thuộc cùng một ứng dụng WeChat.', modes: { open: { title: 'Open Platform', description: 'Dùng quét QR để ủy quyền ngoài WeChat.', appIdLabel: 'App ID Open Platform', appIdPlaceholder: 'WeChat Open Platform App ID', appSecretLabel: 'App Secret Open Platform', appSecretPlaceholder: 'WeChat Open Platform App Secret' }, mp: { title: 'Official Account', description: 'Dùng ủy quyền Official Account trong WeChat.', appIdLabel: 'App ID Official Account', appIdPlaceholder: 'WeChat Official Account App ID', appSecretLabel: 'App Secret Official Account', appSecretPlaceholder: 'WeChat Official Account App Secret' }, mobile: { title: 'Mobile App', description: 'Dùng OAuth Mobile App của WeChat cho desktop/mobile app.', appIdLabel: 'Mobile App ID', appIdPlaceholder: 'WeChat Mobile App ID', appSecretLabel: 'Mobile App Secret', appSecretPlaceholder: 'WeChat Mobile App Secret' } } } },
      subscriptions: { searchDeviceCode: 'Tìm theo device code...', quotaEndsInMinutes: 'Kết thúc sau {minutes} phút', quotaEndsInHoursMinutes: 'Kết thúc sau {hours} giờ {minutes} phút', quotaEndsInDaysHours: 'Kết thúc sau {days} ngày {hours} giờ' },
      users: { sortBy: 'Sắp xếp theo', sortCurrentPageOnly: 'Sắp xếp chỉ áp dụng cho trang hiện tại', columnAlwaysVisible: 'Luôn hiển thị', passwordCopied: 'Đã copy mật khẩu', columns: { usageOpenAI: 'Mức dùng OpenAI', usageAnthropic: 'Mức dùng Anthropic', usageGemini: 'Mức dùng Gemini', usageAntigravity: 'Mức dùng Antigravity' } }
    },
    payment: { admin: { colDeviceCode: 'Mã thiết bị' } }
  },
  ko: {
    admin: {
      affiliates: { records: { payAmount: '결제 금액' } },
      channelMonitor: { form: { apiMode: 'API 모드', apiModeChatCompletions: 'Chat Completions', apiModeChatCompletionsHint: '이 모니터에 OpenAI 호환 /chat/completions 요청을 보냅니다.', apiModeResponses: 'Responses', apiModeResponsesHint: '이 모니터에 OpenAI 호환 /responses 요청을 보냅니다.' } },
      channels: { emptyModelsInPricing: '가격 설정에 모델이 없습니다', noGroupsSelected: '선택된 그룹이 없습니다', form: { syncLatestModels: '최신 모델 동기화', syncingModels: '모델 동기화 중...', syncModelsSuccess: '모델 목록이 동기화되었습니다', syncModelsError: '모델 목록 동기화 실패', syncModelsAlreadyUpToDate: '모델 목록이 이미 최신입니다' } },
      ops: { autoRefreshRemaining: '{seconds}초 후 새로고침', runtime: { metricThresholds: '지표 임계값', metricThresholdsHint: '런타임 대시보드와 알림에 사용하는 경고 임계값을 조정합니다.', requestErrorRateMaxPercent: '요청 오류율 최대값 (%)', requestErrorRateMaxPercentHint: '클라이언트 요청 실패율이 이 비율을 넘으면 알림을 보냅니다.', upstreamErrorRateMaxPercent: '업스트림 오류율 최대값 (%)', upstreamErrorRateMaxPercentHint: '업스트림/제공자 실패율이 이 비율을 넘으면 알림을 보냅니다.', ttftP99MaxMs: 'TTFT P99 최대값 (ms)', ttftP99MaxMsHint: '첫 토큰 P99 지연 시간이 이 값보다 높으면 알림을 보냅니다.', slaMinPercent: '최소 SLA (%)', slaMinPercentHint: '가용성이 이 비율보다 낮으면 알림을 보냅니다.' } },
      redeem: { batchUpdate: '일괄 업데이트', batchUpdateTitle: '리딤 코드 일괄 업데이트', batchUpdateSuccess: '{count}개의 리딤 코드를 업데이트했습니다', batchFields: { status: '상태', group: '그룹', expiresAt: '만료', notes: '메모' }, batchNotesPlaceholder: '선택한 코드에 대한 선택 메모', clearGroup: '그룹 지우기', clearSelection: '선택 해제', codeExpiry: '코드 만료', customExpiry: '사용자 지정 만료', customExpiryDays: '사용자 지정 일수', expiryPresetDays: '{days}일', neverExpires: '만료 없음', noBatchFieldsSelected: '업데이트할 필드를 하나 이상 선택하세요', selectCodesFirst: '먼저 리딤 코드를 선택하세요', selectedCount: '{count}개 선택됨', expiryDaysRequired: '유효한 일수를 입력하세요', failedToBatchUpdate: '리딤 코드 일괄 업데이트 실패', usagePolicy: '사용 정책', usagePolicyHint: '단일 사용은 코드 하나가 전체에서 한 번만 사용됩니다. 사용자당 1회는 캠페인 범위를 공유할 수 있습니다.', usageScope: '캠페인 범위', usageScopePlaceholder: '선택적 공유 캠페인 키', maxTotalUses: '총 사용 한도', maxTotalUsesHint: '0은 총 사용 횟수 무제한을 의미합니다', maxTotalUsesInvalid: '총 사용 한도는 0 이상이어야 합니다', maxUsesPerUser: '사용자당 한도', usagePolicies: { single_use: '단일 사용', once_per_user: '사용자당 1회' }, columns: { expiresAt: '만료일', usagePolicy: '정책', usedCount: '사용됨' } },
      riskControl: { action: { keywordBlock: '키워드 차단' }, keywordBlockingMode: '키워드 차단 모드', keywordModeKeywordOnly: '키워드만', keywordModeKeywordOnlyDesc: '설정된 키워드가 일치할 때만 요청을 차단합니다.', keywordModeKeywordOnlyNotice: '키워드 전용 모드에서는 감사 API를 호출하지 않습니다.', keywordModeApiOnly: '감사 API만', keywordModeApiOnlyDesc: '키워드 사전 차단 없이 감사 API를 사용합니다.', keywordModeApiOnlyNotice: 'API 전용 모드에서는 키워드 규칙이 적용되지 않습니다.', keywordModeKeywordAndApi: '키워드 + 감사 API', keywordModeKeywordAndApiDesc: '먼저 키워드 사전 차단을 적용한 뒤 남은 요청에 감사 API를 사용합니다.', blockedKeywords: '차단 키워드', blockedKeywordCount: '차단 키워드 {count}개', blockedKeywordsDescription: '이 키워드를 포함한 요청은 업스트림에 도달하기 전에 차단됩니다.', blockedKeywordsLimit: '최대 {max}개, 한 줄에 하나씩 입력', blockedKeywordsModeWarning: '{mode} 모드에서 키워드 차단이 활성화되어 있습니다', blockedKeywordsPlaceholder: '한 줄에 키워드 또는 문구 하나', blockedKeywordsPreBlockHint: '키워드가 일치하면 사전 차단 경로에서 즉시 거부됩니다.', defaultBlockMessage: '위험 제어 규칙에 의해 요청이 차단되었습니다.' },
      settings: { authSourceDefaults: { sources: { google: { title: 'Google 가입', description: 'Google OAuth 가입 사용자의 기본 할당량입니다.' }, github: { title: 'GitHub 가입', description: 'GitHub OAuth 가입 사용자의 기본 할당량입니다.' } } }, emailOAuthSettings: { title: '이메일 OAuth 설정', description: '이메일 계정의 Google 및 GitHub OAuth 로그인을 설정합니다.', googleSetupGuide: 'Google Cloud Console에서 OAuth 클라이언트를 만들고 아래 콜백 URL을 추가하세요.', googleHint: 'Google OAuth 로그인과 계정 연결에 사용됩니다.', githubSetupPrefix: 'GitHub OAuth App을 만들고 콜백 URL을 ', githubSetupSuffix: '(으)로 설정하세요.', githubHint: 'GitHub OAuth 로그인과 계정 연결에 사용됩니다.', secretConfiguredPlaceholder: 'Secret이 설정되어 있습니다. 유지하려면 비워 두세요.', callbackUrlSetAndCopied: '콜백 URL을 생성하고 복사했습니다' }, emailTemplates: { title: '이메일 템플릿', description: '언어별 이메일 제목과 HTML 템플릿을 편집합니다.', event: '이벤트', locale: '언어', localeZh: '중국어', localeEn: '영어', subject: '제목', subjectPlaceholder: '이메일 제목', html: 'HTML 본문', htmlPlaceholder: '이메일 HTML 내용', placeholders: '플레이스홀더', placeholdersHelp: '플레이스홀더를 클릭하면 템플릿에 넣을 수 있도록 복사됩니다.', preview: '미리보기', previewing: '미리보는 중...', livePreview: '실시간 미리보기', noPreview: '미리보기가 없습니다', previewSecurityHint: '미리보기는 렌더링 전에 정리됩니다.', customized: '사용자 지정됨', empty: '이벤트와 언어를 선택해 템플릿을 편집하세요.', save: '템플릿 저장', saving: '저장 중...', saveSuccess: '템플릿이 저장되었습니다', restoreOfficial: '공식 템플릿 복원', restoring: '복원 중...', restoreConfirm: '공식 템플릿을 복원할까요? 사용자 지정 내용이 대체됩니다.', restoreSuccess: '공식 템플릿이 복원되었습니다', placeholderCopied: '플레이스홀더가 복사되었습니다', validationRequired: '제목과 HTML 본문은 필수입니다', ...emailTemplateMeta.ko }, wechatConnect: { browserRedirectUrlLabel: '브라우저 리디렉션 URL', browserRedirectUrlHint: '일반 브라우저에서 OAuth가 완료된 후 이동할 주소입니다.', unionIdWarning: '같은 WeChat Open Platform 계정에 연결되어 있지 않으면 UnionID를 사용할 수 없을 수 있습니다.', mpMobileConflict: 'Official Account와 Mobile App은 같은 WeChat 애플리케이션이 아니라면 동일한 AppID/AppSecret을 공유하지 않는 것이 좋습니다.', modes: { open: { title: 'Open Platform', description: 'WeChat 외부에서 QR 코드 인증을 사용합니다.', appIdLabel: 'Open Platform App ID', appIdPlaceholder: 'WeChat Open Platform App ID', appSecretLabel: 'Open Platform App Secret', appSecretPlaceholder: 'WeChat Open Platform App Secret' }, mp: { title: 'Official Account', description: 'WeChat 내부에서 Official Account 인증을 사용합니다.', appIdLabel: 'Official Account App ID', appIdPlaceholder: 'WeChat Official Account App ID', appSecretLabel: 'Official Account App Secret', appSecretPlaceholder: 'WeChat Official Account App Secret' }, mobile: { title: 'Mobile App', description: '데스크톱/모바일 앱 로그인에 WeChat Mobile App OAuth를 사용합니다.', appIdLabel: 'Mobile App ID', appIdPlaceholder: 'WeChat Mobile App ID', appSecretLabel: 'Mobile App Secret', appSecretPlaceholder: 'WeChat Mobile App Secret' } } } },
      subscriptions: { searchDeviceCode: '디바이스 코드로 검색...', quotaEndsInMinutes: '{minutes}분 후 종료', quotaEndsInHoursMinutes: '{hours}시간 {minutes}분 후 종료', quotaEndsInDaysHours: '{days}일 {hours}시간 후 종료' },
      users: { sortBy: '정렬 기준', sortCurrentPageOnly: '정렬은 현재 페이지에만 적용됩니다', columnAlwaysVisible: '항상 표시', passwordCopied: '비밀번호가 복사되었습니다', columns: { usageOpenAI: 'OpenAI 사용량', usageAnthropic: 'Anthropic 사용량', usageGemini: 'Gemini 사용량', usageAntigravity: 'Antigravity 사용량' } }
    },
    payment: { admin: { colDeviceCode: '디바이스 코드' } }
  }
} as const satisfies Record<string, LocalePatch>

const adminLocaleMissingEnPatches = {
  en: {
    admin: {
      dashboard: {
        totalApiKeys: 'Total API Keys',
        activeApiKeys: 'Active API Keys',
        totalUsers: 'Total Users',
        totalAccounts: 'Total Accounts',
        activeAccounts: 'Active Accounts',
        totalRequests: 'Total Requests',
        todayCost: 'Today\'s Cost',
        totalCost: 'Total Cost',
        input: 'Input',
        output: 'Output',
        averageTime: 'Average Time',
        cache: 'Cache',
        last7Days: 'Last 7 Days',
        noUsageRecords: 'No usage records',
        startUsingApi: 'After you start using the API, usage history will be shown here.',
        viewAllUsage: 'View All',
        manageUsers: 'Manage Users',
        viewUserAccounts: 'View and manage user accounts',
        manageAccounts: 'Manage Accounts',
        configureAiAccounts: 'Configure AI Platform Accounts',
        systemSettings: 'System Settings',
        configureSystem: 'Configure System Settings',
      },
      users: {
        deleteConfirmMessage: 'Are you sure you want to delete user \'{email}\'? This action cannot be undone.',
        searchPlaceholder: 'Fuzzy search email/username/notes/API Key/redeem code...',
        roleFilter: 'Role Filter',
        statusFilter: 'Status Filter',
        allStatuses: 'All Statuses',
        form: {
          emailLabel: 'Email',
          emailPlaceholder: 'Please enter email',
          usernameLabel: 'Username',
          usernamePlaceholder: 'Please enter username (optional)',
          notesLabel: 'Notes',
          notesPlaceholder: 'Please enter notes (admin-only)',
          notesHint: 'This note is only visible to administrators',
          passwordLabel: 'Password',
          passwordPlaceholder: 'Please enter password (leave empty to keep unchanged)',
          roleLabel: 'Role',
          selectRole: 'Select role',
          balanceLabel: 'Balance',
          concurrencyLabel: 'Concurrency',
          statusLabel: 'Status',
          selectStatus: 'Select status',
        },
        adjustBalance: 'Adjust Balance',
        adjustConcurrency: 'Adjust Concurrency',
        adjustmentAmount: 'Adjustment Amount',
        adjustmentAmountHint: 'Positive values increase, negative values decrease',
        currentConcurrency: 'Current Concurrency',
        saving: 'Saving...',
        noUsers: 'No users',
        noUsersDescription: 'Create your first user to start using the system.',
        userCreatedSuccess: 'User created successfully',
        userUpdatedSuccess: 'User updated successfully',
        userDeletedSuccess: 'User deleted successfully',
        balanceAdjustedSuccess: 'Balance adjusted successfully',
        concurrencyAdjustedSuccess: 'Concurrency adjusted successfully',
        failedToSave: 'Failed to save user',
        failedToAdjust: 'Adjustment failed',
      },
      groups: {
        columns: {
          exclusive: 'Exclusive',
          priority: 'Priority',
          apiKeys: 'API Key Count',
        },
        form: {
          nameLabel: 'Group Name',
          namePlaceholder: 'Please enter group name',
          descriptionLabel: 'Description',
          descriptionPlaceholder: 'Please enter a description (optional)',
          rateMultiplierLabel: 'Rate Multiplier',
          rateMultiplierHint: '1.0 = standard rate, 0.5 = half price, 2.0 = double price',
          exclusiveLabel: 'Exclusive Group',
          exclusiveHint: 'Exclusive group, can be manually assigned to users',
          platformLabel: 'Platform Restriction',
          platformPlaceholder: 'Select platform (leave empty for no restriction)',
          accountsLabel: 'Assigned Accounts',
          accountsPlaceholder: 'Select accounts (leave empty for no restriction)',
          priorityLabel: 'Priority',
          priorityHint: 'Smaller values indicate higher priority, used for account scheduling',
          statusLabel: 'Status',
        },
        exclusiveObj: {
          yes: 'Yes',
          no: 'No',
        },
        saving: 'Saving...',
        noGroups: 'No groups',
        noGroupsDescription: 'Create groups to better manage API Keys and rates.',
        groupCreatedSuccess: 'Group created successfully',
        groupUpdatedSuccess: 'Group updated successfully',
        groupDeletedSuccess: 'Group deleted successfully',
        exclusiveFilter: 'Exclusive',
      },
      accounts: {
        deleteConfirmMessage: 'Are you sure you want to delete account \'{name}\'?',
        refreshCookie: 'Refresh Cookie',
        testAccount: 'Test Account',
        types: {
          api_key: 'API Key',
          cookie: 'Cookie',
        },
        form: {
          nameLabel: 'Account Name',
          namePlaceholder: 'Please enter account name',
          platformLabel: 'Platform',
          selectPlatform: 'Select platform',
          typeLabel: 'Type',
          selectType: 'Select type',
          credentialsLabel: 'Credentials',
          credentialsPlaceholder: 'Please enter Cookie or API Key',
          priorityLabel: 'Priority',
          priorityHint: 'Smaller values indicate higher priority',
          weightLabel: 'Weight',
          weightHint: 'Weight value used for load balancing',
          statusLabel: 'Status',
        },
        filters: {
          platform: 'Platform',
          allPlatforms: 'All Platforms',
          type: 'Type',
          allTypes: 'All Types',
          status: 'Status',
          allStatuses: 'All Statuses',
        },
        saving: 'Saving...',
        refreshing: 'Refreshing...',
        noAccounts: 'No accounts',
        noAccountsDescription: 'Add AI platform accounts to start using the API gateway.',
        accountCreatedSuccess: 'Account added successfully',
        accountUpdatedSuccess: 'Account updated successfully',
        accountDeletedSuccess: 'Account deleted successfully',
        cookieRefreshedSuccess: 'Cookie refreshed successfully',
        testSuccess: 'Account test passed',
        failedToSave: 'Failed to save account',
      },
      proxies: {
        deleteConfirmMessage: 'Are you sure you want to delete proxy \'{name}\'?',
        testProxy: 'Test Proxy',
        columns: {
          nameLabel: 'Name',
          namePlaceholder: 'Please enter proxy name',
          protocolLabel: 'Protocol',
          selectProtocol: 'Select protocol',
          hostLabel: 'Host',
          hostPlaceholder: 'Please enter host address',
          portLabel: 'Port',
          portPlaceholder: 'Please enter port',
          usernameLabel: 'Username (optional)',
          usernamePlaceholder: 'Please enter username',
          passwordLabel: 'Password (optional)',
          passwordPlaceholder: 'Please enter password',
          priorityLabel: 'Priority',
          statusLabel: 'Status',
        },
        filters: {
          protocol: 'Protocol',
          allProtocols: 'All Protocols',
          status: 'Status',
          allStatuses: 'All Statuses',
        },
        saving: 'Saving...',
        testing: 'Testing...',
        noProxies: 'No proxies',
        noProxiesDescription: 'Add proxy servers to improve API access stability.',
        proxyCreatedSuccess: 'Proxy added successfully',
        proxyUpdatedSuccess: 'Proxy updated successfully',
        proxyDeletedSuccess: 'Proxy deleted successfully',
        testSuccess: 'Proxy test passed',
        failedToSave: 'Failed to save proxy',
      },
      redeem: {
        columns: {
          createdAt: 'Created At',
        },
        form: {
          typeLabel: 'Type',
          selectType: 'Select type',
          valueLabel: 'Value',
          valuePlaceholder: 'Please enter value',
          balanceHint: 'Balance amount (USD)',
          concurrencyHint: 'Concurrency increment',
          countLabel: 'Count',
          countPlaceholder: 'Please enter count',
          countHint: 'Number of redeem codes to generate',
          prefixLabel: 'Prefix (optional)',
          prefixPlaceholder: 'e.g.: GIFT',
          expiresLabel: 'Expiration time (optional)',
        },
        filters: {
          type: 'Type',
          allTypes: 'All Types',
          status: 'Status',
          allStatuses: 'All Statuses',
          search: 'Search redeem codes',
        },
        copyCode: 'Copy',
        disableCode: 'Disable',
        enableCode: 'Enable',
        deleteConfirmMessage: 'Are you sure you want to delete this redeem code?',
        noCodes: 'No redeem codes',
        noCodesDescription: 'Generate redeem codes to distribute balance or concurrency to users.',
        codesGeneratedSuccess: 'Redeem codes generated successfully, {count} total',
        codeDisabledSuccess: 'Redeem code disabled',
        codeEnabledSuccess: 'Redeem code enabled',
        codeDeletedSuccess: 'Redeem code deleted successfully',
        failedToUpdate: 'Failed to update redeem code',
      },
    },
  },
} as const satisfies Record<string, LocalePatch>

const adminLocaleCoveragePatches = {
  en: {
    admin: {
      accounts: {
        gemini: { oauthType: { googleOneAccount: 'Google One account' } }
      },
      affiliates: { records: { deviceCode: 'Device Code' } },
      groups: {
        failedToSave: 'Failed to save group',
        accountFilter: {
          title: 'Account filters',
          disabled: 'Disabled',
          requireOAuthOnly: 'Require OAuth-only accounts',
          oauthOnlyEnabled: 'OAuth-only enabled',
          requirePrivacySet: 'Require privacy setting',
          privacySetEnabled: 'Privacy setting enabled'
        },
        copyAccounts: { optionLabel: '{name} · {platform}' }
      },
      ops: {
        result: 'Result',
        timeRange: { custom: 'Custom' },
        customTimeRange: { startTime: 'Start time', endTime: 'End time' },
        systemLogs: {
          allLevels: 'All levels',
          stacktraceLevel: 'Stacktrace level',
          keyId: 'Key ID',
          caller: 'Caller',
          sampling: 'Sampling',
          noLogs: 'No system logs',
          details: 'Details',
          lastWriteError: 'Last write error',
          cleanupCurrentFilter: 'Clean current filters',
          resetConfirm: 'Reset runtime log configuration to defaults?',
          resetSuccess: 'Runtime log configuration reset',
          resetFailed: 'Failed to reset runtime log configuration',
          runtimeSaved: 'Runtime log configuration saved',
          runtimeSaveFailed: 'Failed to save runtime log configuration'
        }
      },
      settings: {
        telegramNotifications: {
          title: 'Telegram notifications',
          description: 'Send operational notifications to a Telegram chat.',
          botToken: 'Bot token',
          botTokenPlaceholder: '123456:ABC...',
          botTokenConfiguredPlaceholder: 'Bot token configured. Leave empty to keep it.',
          botTokenHint: 'Create a bot with @BotFather and paste its token here.',
          botTokenConfiguredHint: 'A bot token is already configured. Enter a new value only if you want to replace it.',
          chatId: 'Chat ID',
          chatIdPlaceholder: 'e.g. -1001234567890',
          chatIdHint: 'Target Telegram user, group, or channel chat ID.',
          notifyNewUser: 'New user registration',
          notifyNewUserHint: 'Notify when a new user registers.',
          notifyPaymentSuccess: 'Payment success',
          notifyPaymentSuccessHint: 'Notify when a payment succeeds.',
          notifyPaymentFailed: 'Payment failure',
          notifyPaymentFailedHint: 'Notify when a payment fails.',
          notifyRefund: 'Refunds',
          notifyRefundHint: 'Notify when refunds are created or updated.',
          notifyBalanceLow: 'Low balance',
          notifyBalanceLowHint: 'Notify when a user balance drops below the configured threshold.',
          notifySubExpired: 'Subscription expiry',
          notifySubExpiredHint: 'Notify when a subscription expires or is about to expire.',
          notifyAccountExpired: 'Account expiry',
          notifyAccountExpiredHint: 'Notify when an upstream account expires.',
          notifyAccountError: 'Account errors',
          notifyAccountErrorHint: 'Notify when an upstream account enters an error state.',
          notifyProxyExpired: 'Proxy expiry',
          notifyProxyExpiredHint: 'Notify when proxies expire or are about to expire.',
          notifyOpsAlert: 'Ops alerts',
          notifyOpsAlertHint: 'Notify when operational alert rules fire.',
          testConnection: 'Send test message',
          testing: 'Sending...',
          testSuccess: 'Telegram test message sent',
          testFailed: 'Failed to send Telegram test message'
        }
      }
    }
  },
  zh: {
    admin: {
      accounts: {
        gemini: { oauthType: { googleOneAccount: 'Google One 账号' } }
      },
      affiliates: { records: { deviceCode: '设备码' } },
      groups: {
        accountFilter: {
          title: '账号过滤',
          disabled: '不限制',
          requireOAuthOnly: '仅允许 OAuth 账号',
          oauthOnlyEnabled: '已要求 OAuth 账号',
          requirePrivacySet: '要求已设置隐私',
          privacySetEnabled: '已要求隐私设置'
        },
        copyAccounts: { optionLabel: '{name} · {platform}' }
      },
      ops: {
        systemLogs: {
          allLevels: '全部级别',
          stacktraceLevel: '堆栈级别',
          keyId: '密钥 ID',
          caller: '调用方',
          sampling: '采样',
          noLogs: '暂无系统日志',
          details: '详情',
          lastWriteError: '最近写入错误',
          cleanupCurrentFilter: '清理当前筛选结果',
          resetConfirm: '重置运行时日志配置为默认值？',
          resetSuccess: '运行时日志配置已重置',
          resetFailed: '重置运行时日志配置失败',
          runtimeSaved: '运行时日志配置已保存',
          runtimeSaveFailed: '保存运行时日志配置失败'
        }
      },
      settings: {
        telegramNotifications: {
          title: 'Telegram 通知',
          description: '将运营通知发送到 Telegram 会话。',
          botToken: 'Bot Token',
          botTokenPlaceholder: '123456:ABC...',
          botTokenConfiguredPlaceholder: 'Bot Token 已配置，留空保持不变。',
          botTokenHint: '通过 @BotFather 创建机器人并填入 Token。',
          botTokenConfiguredHint: '已配置 Bot Token，仅在需要替换时填写新值。',
          chatId: 'Chat ID',
          chatIdPlaceholder: '例如 -1001234567890',
          chatIdHint: '目标 Telegram 用户、群组或频道 Chat ID。',
          notifyNewUser: '新用户注册',
          notifyNewUserHint: '新用户注册时通知。',
          notifyPaymentSuccess: '支付成功',
          notifyPaymentSuccessHint: '支付成功时通知。',
          notifyPaymentFailed: '支付失败',
          notifyPaymentFailedHint: '支付失败时通知。',
          notifyRefund: '退款',
          notifyRefundHint: '创建或更新退款时通知。',
          notifyBalanceLow: '余额不足',
          notifyBalanceLowHint: '用户余额低于阈值时通知。',
          notifySubExpired: '订阅过期',
          notifySubExpiredHint: '订阅过期或即将过期时通知。',
          notifyAccountExpired: '账号过期',
          notifyAccountExpiredHint: '上游账号过期时通知。',
          notifyAccountError: '账号错误',
          notifyAccountErrorHint: '上游账号进入错误状态时通知。',
          notifyProxyExpired: '代理过期',
          notifyProxyExpiredHint: '代理过期或即将过期时通知。',
          notifyOpsAlert: '运维告警',
          notifyOpsAlertHint: '运维告警规则触发时通知。',
          testConnection: '发送测试消息',
          testing: '发送中...',
          testSuccess: 'Telegram 测试消息已发送',
          testFailed: 'Telegram 测试消息发送失败'
        }
      }
    }
  }
} as const satisfies Record<string, LocalePatch>

const staticI18nCoveragePatches = {
  en: {
    common: {
      apply: 'Apply',
      clear: 'Clear',
      creating: 'Creating...',
      rateMultiplier: 'Rate multiplier',
      required: 'Required',
      sending: 'Sending...',
      tryAgain: 'Try again'
    },
    auth: {
      loginAgreement: {
        passwordSignInBlocked: 'Accept the login agreement before signing in with password.',
        registerBlocked: 'Accept the login agreement before creating an account.',
        registerRequired: 'Please read and accept the agreement to register.',
        signInRequired: 'Please read and accept the agreement to sign in.'
      },
      dingtalk: {
        callbackTitle: 'DingTalk sign-in',
        callbackProcessing: 'Processing DingTalk authorization...',
        callbackHint: 'Complete authorization in DingTalk, then return here to continue.',
        callbackMissingToken: 'DingTalk callback did not return an access token.',
        completeRegistration: 'Complete registration',
        completeRegistrationFailed: 'Failed to complete DingTalk registration',
        completing: 'Completing...',
        createAccountTitle: 'Create account with DingTalk',
        invitationRequired: 'An invitation code is required to complete DingTalk registration.',
        registrationDisabledRedirectToBind: 'Registration is disabled. Sign in first, then bind DingTalk from your profile.',
        signIn: 'Sign in with DingTalk'
      }
    },
    keyUsage: {
      dailyDetail: 'Daily usage detail',
      date: 'Date',
      cacheWriteTokens: 'Cache write tokens',
      dateRange90d: '90 Days',
      noDailyUsage: 'No daily usage records'
    },
    userSubscriptions: {
      quotaEndsIn: 'Quota ends in {time}'
    },
    admin: {
      accounts: {
        add: 'Add account',
        fromModel: 'Source model',
        toModel: 'Target model',
        syncUpstreamModels: 'Sync upstream models',
        syncUpstreamModelsLoading: 'Syncing upstream models...',
        syncUpstreamModelsEmpty: 'No upstream models found',
        syncUpstreamModelsSuccess: 'Upstream models synced',
        syncUpstreamModelsNoChanges: 'Upstream models are already up to date',
        syncUpstreamModelsFailed: 'Failed to sync upstream models',
        syncUpstreamModelsError: 'Sync upstream models error: {message}',
        oauth: {
          openai: {
            mobileRefreshTokenAuth: 'Mobile refresh token',
            accessTokenAuth: 'Access token'
          }
        },
        gemini: {
          setupGuide: {
            links: {
              changeCountryAssociation: 'Change country/region association'
            }
          }
        }
      },
      groups: {
        modelsList: {
          selectedTotal: 'Selected {selected} / {total}',
          invertSelection: 'Invert selection'
        }
      },
      users: {
        platformBreakdown: 'Platform breakdown',
        platformOther: 'Other platforms'
      }
    },
    payment: {
      orders: {
        payAmount: 'Pay amount'
      }
    }
  },
  zh: {
    common: {
      apply: '应用',
      clear: '清除',
      creating: '创建中...',
      rateMultiplier: '费率倍数',
      required: '必填',
      sending: '发送中...',
      tryAgain: '重试'
    },
    auth: {
      loginAgreement: {
        passwordSignInBlocked: '请先同意登录协议后再使用密码登录。',
        registerBlocked: '请先同意登录协议后再创建账号。',
        registerRequired: '请阅读并同意协议后注册。',
        signInRequired: '请阅读并同意协议后登录。'
      },
      dingtalk: {
        callbackTitle: '钉钉登录',
        callbackProcessing: '正在处理钉钉授权...',
        callbackHint: '请在钉钉完成授权，然后返回此页面继续。',
        callbackMissingToken: '钉钉回调未返回访问令牌。',
        completeRegistration: '完成注册',
        completeRegistrationFailed: '完成钉钉注册失败',
        completing: '正在完成...',
        createAccountTitle: '使用钉钉创建账号',
        invitationRequired: '需要邀请码才能完成钉钉注册。',
        registrationDisabledRedirectToBind: '当前已关闭注册。请先登录，再到个人资料中绑定钉钉。',
        signIn: '使用钉钉登录'
      }
    },
    keyUsage: {
      dailyDetail: '每日用量明细',
      date: '日期',
      cacheWriteTokens: '缓存写入 Tokens',
      dateRange90d: '90 天',
      noDailyUsage: '暂无每日用量记录'
    },
    userSubscriptions: {
      quotaEndsIn: '额度将在 {time} 后结束'
    },
    admin: {
      accounts: {
        add: '添加账号',
        fromModel: '源模型',
        toModel: '目标模型',
        syncUpstreamModels: '同步上游模型',
        syncUpstreamModelsLoading: '正在同步上游模型...',
        syncUpstreamModelsEmpty: '未发现上游模型',
        syncUpstreamModelsSuccess: '上游模型已同步',
        syncUpstreamModelsNoChanges: '上游模型已是最新',
        syncUpstreamModelsFailed: '同步上游模型失败',
        syncUpstreamModelsError: '同步上游模型出错：{message}',
        oauth: {
          openai: {
            mobileRefreshTokenAuth: '移动端 Refresh Token',
            accessTokenAuth: 'Access Token'
          }
        },
        gemini: {
          setupGuide: {
            links: {
              changeCountryAssociation: '更改国家/地区关联'
            }
          }
        }
      },
      groups: {
        modelsList: {
          selectedTotal: '已选 {selected} / {total}',
          invertSelection: '反选'
        }
      },
      users: {
        platformBreakdown: '平台分布',
        platformOther: '其他平台'
      }
    },
    payment: {
      orders: {
        payAmount: '支付金额'
      }
    }
  },
  vi: {
    common: {
      apply: 'Áp dụng',
      clear: 'Xóa',
      creating: 'Đang tạo...',
      rateMultiplier: 'Hệ số giá',
      required: 'Bắt buộc',
      sending: 'Đang gửi...',
      tryAgain: 'Thử lại'
    },
    auth: {
      loginAgreement: {
        passwordSignInBlocked: 'Hãy chấp nhận thỏa thuận đăng nhập trước khi đăng nhập bằng mật khẩu.',
        registerBlocked: 'Hãy chấp nhận thỏa thuận đăng nhập trước khi tạo tài khoản.',
        registerRequired: 'Vui lòng đọc và chấp nhận thỏa thuận để đăng ký.',
        signInRequired: 'Vui lòng đọc và chấp nhận thỏa thuận để đăng nhập.'
      },
      dingtalk: {
        callbackTitle: 'Đăng nhập DingTalk',
        callbackProcessing: 'Đang xử lý ủy quyền DingTalk...',
        callbackHint: 'Hoàn tất ủy quyền trong DingTalk rồi quay lại đây để tiếp tục.',
        callbackMissingToken: 'Callback DingTalk không trả về access token.',
        completeRegistration: 'Hoàn tất đăng ký',
        completeRegistrationFailed: 'Hoàn tất đăng ký DingTalk thất bại',
        completing: 'Đang hoàn tất...',
        createAccountTitle: 'Tạo tài khoản bằng DingTalk',
        invitationRequired: 'Cần mã mời để hoàn tất đăng ký DingTalk.',
        registrationDisabledRedirectToBind: 'Đăng ký đang bị tắt. Hãy đăng nhập trước rồi liên kết DingTalk trong hồ sơ.',
        signIn: 'Đăng nhập bằng DingTalk'
      }
    },
    keyUsage: {
      dailyDetail: 'Chi tiết sử dụng hằng ngày',
      date: 'Ngày',
      cacheWriteTokens: 'Cache write tokens',
      dateRange90d: '90 ngày',
      noDailyUsage: 'Chưa có bản ghi sử dụng hằng ngày'
    },
    userSubscriptions: {
      quotaEndsIn: 'Quota kết thúc sau {time}'
    },
    admin: {
      accounts: {
        add: 'Thêm tài khoản',
        fromModel: 'Model nguồn',
        toModel: 'Model đích',
        syncUpstreamModels: 'Đồng bộ model upstream',
        syncUpstreamModelsLoading: 'Đang đồng bộ model upstream...',
        syncUpstreamModelsEmpty: 'Không tìm thấy model upstream',
        syncUpstreamModelsSuccess: 'Đã đồng bộ model upstream',
        syncUpstreamModelsNoChanges: 'Model upstream đã là mới nhất',
        syncUpstreamModelsFailed: 'Đồng bộ model upstream thất bại',
        syncUpstreamModelsError: 'Lỗi đồng bộ model upstream: {message}',
        oauth: {
          openai: {
            mobileRefreshTokenAuth: 'Mobile refresh token',
            accessTokenAuth: 'Access token'
          }
        },
        gemini: {
          setupGuide: {
            links: {
              changeCountryAssociation: 'Đổi liên kết quốc gia/khu vực'
            }
          }
        }
      },
      groups: {
        modelsList: {
          selectedTotal: 'Đã chọn {selected} / {total}',
          invertSelection: 'Đảo chọn'
        }
      },
      users: {
        platformBreakdown: 'Phân bổ theo nền tảng',
        platformOther: 'Nền tảng khác'
      }
    },
    payment: {
      orders: {
        payAmount: 'Số tiền thanh toán'
      }
    }
  },
  ko: {
    common: {
      apply: '적용',
      clear: '지우기',
      creating: '생성 중...',
      rateMultiplier: '요율 배수',
      required: '필수',
      sending: '전송 중...',
      tryAgain: '다시 시도'
    },
    auth: {
      loginAgreement: {
        passwordSignInBlocked: '비밀번호로 로그인하기 전에 로그인 약관에 동의하세요.',
        registerBlocked: '계정을 만들기 전에 로그인 약관에 동의하세요.',
        registerRequired: '가입하려면 약관을 읽고 동의해 주세요.',
        signInRequired: '로그인하려면 약관을 읽고 동의해 주세요.'
      },
      dingtalk: {
        callbackTitle: 'DingTalk 로그인',
        callbackProcessing: 'DingTalk 인증을 처리하는 중...',
        callbackHint: 'DingTalk에서 인증을 완료한 뒤 여기로 돌아와 계속하세요.',
        callbackMissingToken: 'DingTalk callback이 access token을 반환하지 않았습니다.',
        completeRegistration: '가입 완료',
        completeRegistrationFailed: 'DingTalk 가입 완료 실패',
        completing: '완료 중...',
        createAccountTitle: 'DingTalk로 계정 만들기',
        invitationRequired: 'DingTalk 가입을 완료하려면 초대 코드가 필요합니다.',
        registrationDisabledRedirectToBind: '가입이 비활성화되어 있습니다. 먼저 로그인한 뒤 프로필에서 DingTalk를 연결하세요.',
        signIn: 'DingTalk로 로그인'
      }
    },
    keyUsage: {
      dailyDetail: '일별 사용량 상세',
      date: '날짜',
      cacheWriteTokens: 'Cache write tokens',
      dateRange90d: '90일',
      noDailyUsage: '일별 사용량 기록이 없습니다'
    },
    userSubscriptions: {
      quotaEndsIn: '{time} 후 quota 종료'
    },
    admin: {
      accounts: {
        add: '계정 추가',
        fromModel: '원본 모델',
        toModel: '대상 모델',
        syncUpstreamModels: '업스트림 모델 동기화',
        syncUpstreamModelsLoading: '업스트림 모델 동기화 중...',
        syncUpstreamModelsEmpty: '업스트림 모델을 찾을 수 없습니다',
        syncUpstreamModelsSuccess: '업스트림 모델이 동기화되었습니다',
        syncUpstreamModelsNoChanges: '업스트림 모델이 이미 최신입니다',
        syncUpstreamModelsFailed: '업스트림 모델 동기화 실패',
        syncUpstreamModelsError: '업스트림 모델 동기화 오류: {message}',
        oauth: {
          openai: {
            mobileRefreshTokenAuth: 'Mobile refresh token',
            accessTokenAuth: 'Access token'
          }
        },
        gemini: {
          setupGuide: {
            links: {
              changeCountryAssociation: '국가/지역 연결 변경'
            }
          }
        }
      },
      groups: {
        modelsList: {
          selectedTotal: '{selected} / {total} 선택됨',
          invertSelection: '선택 반전'
        }
      },
      users: {
        platformBreakdown: '플랫폼별 분포',
        platformOther: '기타 플랫폼'
      }
    },
    payment: {
      orders: {
        payAmount: '결제 금액'
      }
    }
  }
} as const satisfies Record<string, LocalePatch>


const staticI18nCoverageHotfixPatches = {
  en: {
    auth: {
      wechatConnect: {
        mobileOnlyHint: 'This site is configured for WeChat Mobile App login only. Continue from the native app through the WeChat SDK.'
      }
    },
    keyUsage: {
      windowUnits: {
        day: 'D',
        week: 'W',
        month: 'M'
      }
    },
    admin: {
      accounts: {
        messages: {
          accountCreated: 'Account created successfully'
        },
        status: {
          overageActive: 'Overage',
          overageExhausted: 'Overage exhausted',
          overageExhaustedUntil: 'Overage exhausted; expected to recover at {time}'
        },
        gemini: {
          oauthType: {
            badges: {
              recommendedPersonal: 'Recommended for personal users',
              enterpriseUser: 'Enterprise users'
            }
          }
        }
      }
    }
  },
  zh: {
    auth: {
      wechatConnect: {
        mobileOnlyHint: '当前站点仅配置了微信移动应用登录，请在原生应用中通过微信 SDK 继续。'
      }
    },
    keyUsage: {
      windowUnits: {
        day: 'D',
        week: 'W',
        month: 'M'
      }
    },
    admin: {
      accounts: {
        messages: {
          accountCreated: '账号创建成功'
        },
        status: {
          overageActive: '超额使用',
          overageExhausted: '超额用量已耗尽',
          overageExhaustedUntil: '超额用量已耗尽，预计 {time} 恢复'
        },
        gemini: {
          oauthType: {
            badges: {
              recommendedPersonal: '推荐个人用户',
              enterpriseUser: '企业用户'
            }
          }
        }
      }
    }
  },
  vi: {
    admin: {
      accounts: {
        messages: {
          accountCreated: 'Tạo tài khoản thành công'
        }
      }
    }
  },
  ko: {
    admin: {
      accounts: {
        messages: {
          accountCreated: '계정이 생성되었습니다'
        }
      }
    }
  }
} as const satisfies Record<string, LocalePatch>

const zhBaselineLocaleCoveragePatches = {
  vi: {
    usage: {
      cyber: 'Chính sách an toàn',
      errors: {
        categories: {
          cyber: 'Chính sách an toàn'
        }
      }
    },
    admin: {
      riskControl: {
        cyberPolicyExcludeBan: 'Không tính cyber_policy vào số lần cấm',
        cyberPolicyExcludeBanHint: 'Khi bật, các lần bị chặn bởi cyber_policy sẽ không còn tính vào số lần vi phạm để tự động cấm: lần hit hiện tại không kích hoạt xét cấm và các bản ghi lịch sử cũng bị loại khỏi bộ đếm rolling. Nhật ký risk control và email thông báo vẫn giữ nguyên.',
        violationNotCounted: 'Không tính vào lệnh cấm',
        action: {
          cyberPolicy: 'Chính sách an toàn'
        }
      },
      channelMonitor: {
        form: {
          jitterSeconds: 'Độ lệch ngẫu nhiên (± giây)',
          jitterSecondsHint: 'Mỗi lần kiểm tra sẽ chạy tại khoảng thời gian ± một độ lệch ngẫu nhiên trong giá trị này; 0 nghĩa là cố định. Khoảng cách trừ độ lệch phải ≥ 15 giây'
        }
      },
      accounts: {
        columns: {
          id: 'ID'
        },
        openaiQuotaReset: {
          count: 'Lượt',
          reset: 'Đặt lại',
          countTooltipLoad: 'Bấm để tải số lượt reset còn lại',
          countTooltipRefresh: 'Bấm để làm mới số lượt reset còn lại',
          resetTooltipReady: 'Dùng 1 lượt reset để khôi phục ngay cửa sổ hiện tại',
          resetTooltipNeedQuery: 'Bấm “Lượt” trước để tải số lượt reset còn lại',
          resetTooltipNoCredits: 'Không còn lượt reset khả dụng',
          noCreditsAvailable: 'Không còn lượt reset khả dụng',
          resetSuccess: 'Đã reset {windows} cửa sổ',
          confirmTitle: 'Xác nhận đặt lại giới hạn tuần',
          confirmMessage: 'Thao tác này sẽ dùng 1 lượt reset để khôi phục ngay cửa sổ hiện tại ({count} lượt còn lại). Không thể hoàn tác. Tiếp tục?'
        }
      },
      settings: {
        features: {
          riskControl: {
            cyberSessionBlock: 'Tự động chặn phiên cyber',
            cyberSessionBlockHint: 'Khi bật, phiên bị upstream cyber_policy chặn sẽ bị chặn cục bộ trong thời gian TTL và không còn gửi lên upstream. Chỉ chặn phiên vi phạm; các phiên khác trên cùng key không bị ảnh hưởng.',
            cyberSessionBlockTTL: 'TTL chặn (giây)'
          }
        },
        gatewayForwarding: {
          claudeOAuthSystemPromptInjection: 'Tiêm Claude OAuth System',
          claudeOAuthSystemPromptInjectionHint: 'Tiêm các system block theo dạng Claude Code cho request Claude OAuth từ client không phải Claude Code. Mặc định bật.',
          claudeOAuthSystemPrompt: 'Prompt mở rộng Claude OAuth',
          claudeOAuthSystemPromptPlaceholder: 'Để trống để dùng prompt mở rộng Claude Code tích hợp.',
          claudeOAuthSystemPromptHint: 'Tương thích cấu hình cũ: chỉ điều khiển system block thứ ba được tiêm.',
          claudeOAuthSystemPromptBlocks: 'Claude OAuth System Blocks',
          claudeOAuthSystemPromptBlocksPlaceholder: 'Để trống để dùng 3 block tích hợp. Hỗ trợ mảng hoặc {"blocks": [...]}.',
          claudeOAuthSystemPromptBlocksHint: 'Mỗi block sẽ được lưu dưới dạng JSON có enabled, type, text và cache_control tùy chọn. {billing_header} được tạo động theo request; prompt nhận diện Claude Code và prompt mở rộng có thể chỉnh trực tiếp hoặc khôi phục bằng preset.',
          systemBlockTitle: 'System Block {index}',
          systemBlockPreset: 'Preset',
          systemBlockPresetBilling: 'Billing Header',
          systemBlockPresetIdentity: 'Prompt nhận diện Claude Code',
          systemBlockPresetExpansion: 'Prompt mở rộng Claude Code',
          systemBlockPresetCustom: 'Tùy chỉnh',
          systemBlockType: 'Loại',
          systemBlockTypeText: 'Văn bản',
          systemBlockText: 'Nội dung',
          systemBlockCacheControl: 'Cache Control',
          systemBlockHide: 'Ẩn chi tiết block',
          systemBlockShow: 'Hiện chi tiết block',
          addSystemBlock: 'Thêm block',
          resetSystemBlocks: 'Khôi phục mặc định',
          cacheTTL5m: '5 phút',
          cacheTTL1h: '1 giờ'
        }
      }
    }
  },
  ko: {
    usage: {
      cyber: '보안 정책',
      errors: {
        categories: {
          cyber: '보안 정책'
        }
      }
    },
    admin: {
      riskControl: {
        cyberPolicyExcludeBan: 'cyber_policy 차단을 금지 횟수에서 제외',
        cyberPolicyExcludeBanHint: '활성화하면 cyber_policy 차단은 자동 차단 위반 횟수에 더 이상 포함되지 않습니다. 해당 hit 자체로 차단 판단을 하지 않고, rolling count에서도 과거 기록을 제외합니다. 위험 제어 로그와 알림 이메일은 그대로 유지됩니다.',
        violationNotCounted: '차단 횟수 미포함',
        action: {
          cyberPolicy: '보안 정책'
        }
      },
      channelMonitor: {
        form: {
          jitterSeconds: '무작위 지터 (±초)',
          jitterSecondsHint: '각 점검은 간격에 이 값 범위의 무작위 오프셋을 더하거나 빼서 실행됩니다. 0은 고정 간격을 의미합니다. 간격 - 지터는 15초 이상이어야 합니다'
        }
      },
      accounts: {
        columns: {
          id: 'ID'
        },
        openaiQuotaReset: {
          count: '횟수',
          reset: '재설정',
          countTooltipLoad: '남은 재설정 횟수를 불러오려면 클릭하세요',
          countTooltipRefresh: '남은 재설정 횟수를 새로고침하려면 클릭하세요',
          resetTooltipReady: '재설정 횟수 1회를 사용해 현재 창을 즉시 복구합니다',
          resetTooltipNeedQuery: '먼저 “횟수”를 클릭해 남은 재설정 횟수를 불러오세요',
          resetTooltipNoCredits: '사용 가능한 재설정 횟수가 없습니다',
          noCreditsAvailable: '사용 가능한 재설정 횟수가 없습니다',
          resetSuccess: '{windows}개 창을 재설정했습니다',
          confirmTitle: '주간 제한 재설정 확인',
          confirmMessage: '재설정 횟수 1회를 사용해 현재 창을 즉시 복구합니다(남은 {count}회). 이 작업은 되돌릴 수 없습니다. 계속하시겠습니까?'
        }
      },
      settings: {
        features: {
          riskControl: {
            cyberSessionBlock: 'cyber 세션 자동 차단',
            cyberSessionBlockHint: '활성화하면 upstream cyber_policy에 의해 차단된 세션은 TTL 동안 로컬에서 차단되어 upstream으로 전달되지 않습니다. 해당 세션만 차단하며 같은 key의 다른 세션에는 영향을 주지 않습니다.',
            cyberSessionBlockTTL: '차단 TTL(초)'
          }
        },
        gatewayForwarding: {
          claudeOAuthSystemPromptInjection: 'Claude OAuth System 주입',
          claudeOAuthSystemPromptInjectionHint: 'Claude Code가 아닌 클라이언트의 Claude OAuth 요청에 Claude Code 형태의 system blocks를 주입합니다. 기본값은 활성화입니다.',
          claudeOAuthSystemPrompt: 'Claude OAuth 확장 프롬프트',
          claudeOAuthSystemPromptPlaceholder: '비워 두면 내장 Claude Code 확장 프롬프트를 사용합니다.',
          claudeOAuthSystemPromptHint: '이전 설정과의 호환성: 세 번째로 주입되는 system block만 제어합니다.',
          claudeOAuthSystemPromptBlocks: 'Claude OAuth System Blocks',
          claudeOAuthSystemPromptBlocksPlaceholder: '비워 두면 내장 3개 blocks를 사용합니다. 배열 또는 {"blocks": [...]} 형식을 지원합니다.',
          claudeOAuthSystemPromptBlocksHint: '각 block은 enabled, type, text 및 선택적 cache_control을 포함한 JSON으로 저장됩니다. {billing_header}는 요청마다 동적으로 생성됩니다. Claude Code 신원 프롬프트와 확장 프롬프트는 직접 편집하거나 preset으로 기본값을 복원할 수 있습니다.',
          systemBlockTitle: 'System Block {index}',
          systemBlockPreset: '프리셋',
          systemBlockPresetBilling: 'Billing Header',
          systemBlockPresetIdentity: 'Claude Code 신원 프롬프트',
          systemBlockPresetExpansion: 'Claude Code 확장 프롬프트',
          systemBlockPresetCustom: '사용자 지정',
          systemBlockType: '유형',
          systemBlockTypeText: '텍스트',
          systemBlockText: '내용',
          systemBlockCacheControl: 'Cache Control',
          systemBlockHide: 'block 상세 숨기기',
          systemBlockShow: 'block 상세 보기',
          addSystemBlock: 'block 추가',
          resetSystemBlocks: '기본값 복원',
          cacheTTL5m: '5분',
          cacheTTL1h: '1시간'
        }
      }
    }
  }
} as const satisfies Record<string, LocalePatch>

export const adminLocalePatches = mergeLocalePatch(
  mergeLocalePatch(
    adminLocalePatchBase as unknown as Record<string, LocalePatch>,
    adminLocaleMissingEnPatches
  ),
  adminLocaleCoveragePatches
)

mergeLocalePatch(adminLocalePatches.vi, zhBaselineLocaleCoveragePatches.vi)
mergeLocalePatch(adminLocalePatches.ko, zhBaselineLocaleCoveragePatches.ko)

for (const locale of ['en', 'zh', 'vi', 'ko'] as const) {
  mergeLocalePatch(adminLocalePatches[locale], staticI18nCoveragePatches[locale])
}

for (const locale of ['en', 'zh', 'vi', 'ko'] as const) {
  mergeLocalePatch(adminLocalePatches[locale], staticI18nCoverageHotfixPatches[locale])
}
