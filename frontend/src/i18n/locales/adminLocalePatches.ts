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
          validationRequired: 'Subject and HTML body are required'
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
          title: '邮件模板', description: '编辑多语言邮件标题和 HTML 模板。', event: '事件', locale: '语言', localeZh: '中文', localeEn: '英文', subject: '标题', subjectPlaceholder: '邮件标题', html: 'HTML 正文', htmlPlaceholder: '邮件 HTML 内容', placeholders: '占位符', placeholdersHelp: '点击占位符可复制到模板。', preview: '预览', previewing: '预览中...', livePreview: '实时预览', noPreview: '暂无预览', previewSecurityHint: '预览内容会先清洗再渲染。', customized: '已自定义', empty: '请选择事件和语言后编辑模板。', save: '保存模板', saving: '保存中...', saveSuccess: '模板已保存', restoreOfficial: '恢复官方模板', restoring: '恢复中...', restoreConfirm: '恢复官方模板？自定义内容会被替换。', restoreSuccess: '官方模板已恢复', placeholderCopied: '占位符已复制', validationRequired: '标题和 HTML 正文为必填项'
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
      settings: { authSourceDefaults: { sources: { google: { title: 'Đăng ký Google', description: 'Quota mặc định cho người dùng đăng ký bằng Google OAuth.' }, github: { title: 'Đăng ký GitHub', description: 'Quota mặc định cho người dùng đăng ký bằng GitHub OAuth.' } } }, emailOAuthSettings: { title: 'Cài đặt Email OAuth', description: 'Cấu hình đăng nhập Google và GitHub OAuth cho tài khoản email.', googleSetupGuide: 'Tạo OAuth client trong Google Cloud Console và thêm Callback URL bên dưới.', googleHint: 'Dùng cho đăng nhập Google OAuth và liên kết tài khoản.', githubSetupPrefix: 'Tạo GitHub OAuth App và đặt Callback URL thành ', githubSetupSuffix: '.', githubHint: 'Dùng cho đăng nhập GitHub OAuth và liên kết tài khoản.', secretConfiguredPlaceholder: 'Đã cấu hình secret. Để trống để giữ nguyên.', callbackUrlSetAndCopied: 'Đã tạo và copy Callback URL' }, emailTemplates: { title: 'Mẫu email', description: 'Chỉnh sửa tiêu đề và HTML email theo từng ngôn ngữ.', event: 'Sự kiện', locale: 'Ngôn ngữ', localeZh: 'Tiếng Trung', localeEn: 'Tiếng Anh', subject: 'Tiêu đề', subjectPlaceholder: 'Tiêu đề email', html: 'Nội dung HTML', htmlPlaceholder: 'Nội dung HTML của email', placeholders: 'Biến placeholder', placeholdersHelp: 'Bấm vào placeholder để copy vào mẫu.', preview: 'Xem trước', previewing: 'Đang xem trước...', livePreview: 'Xem trước trực tiếp', noPreview: 'Chưa có bản xem trước', previewSecurityHint: 'Bản xem trước sẽ được sanitize trước khi hiển thị.', customized: 'Đã tùy chỉnh', empty: 'Chọn sự kiện và ngôn ngữ để chỉnh sửa mẫu.', save: 'Lưu mẫu', saving: 'Đang lưu...', saveSuccess: 'Đã lưu mẫu', restoreOfficial: 'Khôi phục mẫu mặc định', restoring: 'Đang khôi phục...', restoreConfirm: 'Khôi phục mẫu mặc định? Nội dung tùy chỉnh sẽ bị thay thế.', restoreSuccess: 'Đã khôi phục mẫu mặc định', placeholderCopied: 'Đã copy placeholder', validationRequired: 'Tiêu đề và nội dung HTML là bắt buộc' }, wechatConnect: { browserRedirectUrlLabel: 'URL chuyển hướng trình duyệt', browserRedirectUrlHint: 'Dùng sau khi OAuth hoàn tất trong trình duyệt thường.', unionIdWarning: 'UnionID có thể không khả dụng nếu tài khoản không cùng WeChat Open Platform.', mpMobileConflict: 'Official Account và Mobile App không nên dùng chung AppID/AppSecret trừ khi thuộc cùng một ứng dụng WeChat.', modes: { open: { title: 'Open Platform', description: 'Dùng quét QR để ủy quyền ngoài WeChat.', appIdLabel: 'App ID Open Platform', appIdPlaceholder: 'WeChat Open Platform App ID', appSecretLabel: 'App Secret Open Platform', appSecretPlaceholder: 'WeChat Open Platform App Secret' }, mp: { title: 'Official Account', description: 'Dùng ủy quyền Official Account trong WeChat.', appIdLabel: 'App ID Official Account', appIdPlaceholder: 'WeChat Official Account App ID', appSecretLabel: 'App Secret Official Account', appSecretPlaceholder: 'WeChat Official Account App Secret' }, mobile: { title: 'Mobile App', description: 'Dùng OAuth Mobile App của WeChat cho desktop/mobile app.', appIdLabel: 'Mobile App ID', appIdPlaceholder: 'WeChat Mobile App ID', appSecretLabel: 'Mobile App Secret', appSecretPlaceholder: 'WeChat Mobile App Secret' } } } },
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
      settings: { authSourceDefaults: { sources: { google: { title: 'Google 가입', description: 'Google OAuth 가입 사용자의 기본 할당량입니다.' }, github: { title: 'GitHub 가입', description: 'GitHub OAuth 가입 사용자의 기본 할당량입니다.' } } }, emailOAuthSettings: { title: '이메일 OAuth 설정', description: '이메일 계정의 Google 및 GitHub OAuth 로그인을 설정합니다.', googleSetupGuide: 'Google Cloud Console에서 OAuth 클라이언트를 만들고 아래 콜백 URL을 추가하세요.', googleHint: 'Google OAuth 로그인과 계정 연결에 사용됩니다.', githubSetupPrefix: 'GitHub OAuth App을 만들고 콜백 URL을 ', githubSetupSuffix: '(으)로 설정하세요.', githubHint: 'GitHub OAuth 로그인과 계정 연결에 사용됩니다.', secretConfiguredPlaceholder: 'Secret이 설정되어 있습니다. 유지하려면 비워 두세요.', callbackUrlSetAndCopied: '콜백 URL을 생성하고 복사했습니다' }, emailTemplates: { title: '이메일 템플릿', description: '언어별 이메일 제목과 HTML 템플릿을 편집합니다.', event: '이벤트', locale: '언어', localeZh: '중국어', localeEn: '영어', subject: '제목', subjectPlaceholder: '이메일 제목', html: 'HTML 본문', htmlPlaceholder: '이메일 HTML 내용', placeholders: '플레이스홀더', placeholdersHelp: '플레이스홀더를 클릭하면 템플릿에 넣을 수 있도록 복사됩니다.', preview: '미리보기', previewing: '미리보는 중...', livePreview: '실시간 미리보기', noPreview: '미리보기가 없습니다', previewSecurityHint: '미리보기는 렌더링 전에 정리됩니다.', customized: '사용자 지정됨', empty: '이벤트와 언어를 선택해 템플릿을 편집하세요.', save: '템플릿 저장', saving: '저장 중...', saveSuccess: '템플릿이 저장되었습니다', restoreOfficial: '공식 템플릿 복원', restoring: '복원 중...', restoreConfirm: '공식 템플릿을 복원할까요? 사용자 지정 내용이 대체됩니다.', restoreSuccess: '공식 템플릿이 복원되었습니다', placeholderCopied: '플레이스홀더가 복사되었습니다', validationRequired: '제목과 HTML 본문은 필수입니다' }, wechatConnect: { browserRedirectUrlLabel: '브라우저 리디렉션 URL', browserRedirectUrlHint: '일반 브라우저에서 OAuth가 완료된 후 이동할 주소입니다.', unionIdWarning: '같은 WeChat Open Platform 계정에 연결되어 있지 않으면 UnionID를 사용할 수 없을 수 있습니다.', mpMobileConflict: 'Official Account와 Mobile App은 같은 WeChat 애플리케이션이 아니라면 동일한 AppID/AppSecret을 공유하지 않는 것이 좋습니다.', modes: { open: { title: 'Open Platform', description: 'WeChat 외부에서 QR 코드 인증을 사용합니다.', appIdLabel: 'Open Platform App ID', appIdPlaceholder: 'WeChat Open Platform App ID', appSecretLabel: 'Open Platform App Secret', appSecretPlaceholder: 'WeChat Open Platform App Secret' }, mp: { title: 'Official Account', description: 'WeChat 내부에서 Official Account 인증을 사용합니다.', appIdLabel: 'Official Account App ID', appIdPlaceholder: 'WeChat Official Account App ID', appSecretLabel: 'Official Account App Secret', appSecretPlaceholder: 'WeChat Official Account App Secret' }, mobile: { title: 'Mobile App', description: '데스크톱/모바일 앱 로그인에 WeChat Mobile App OAuth를 사용합니다.', appIdLabel: 'Mobile App ID', appIdPlaceholder: 'WeChat Mobile App ID', appSecretLabel: 'Mobile App Secret', appSecretPlaceholder: 'WeChat Mobile App Secret' } } } },
      subscriptions: { searchDeviceCode: '디바이스 코드로 검색...', quotaEndsInMinutes: '{minutes}분 후 종료', quotaEndsInHoursMinutes: '{hours}시간 {minutes}분 후 종료', quotaEndsInDaysHours: '{days}일 {hours}시간 후 종료' },
      users: { sortBy: '정렬 기준', sortCurrentPageOnly: '정렬은 현재 페이지에만 적용됩니다', columnAlwaysVisible: '항상 표시', passwordCopied: '비밀번호가 복사되었습니다', columns: { usageOpenAI: 'OpenAI 사용량', usageAnthropic: 'Anthropic 사용량', usageGemini: 'Gemini 사용량', usageAntigravity: 'Antigravity 사용량' } }
    },
    payment: { admin: { colDeviceCode: '디바이스 코드' } }
  }
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

export const adminLocalePatches = mergeLocalePatch(
  adminLocalePatchBase as unknown as Record<string, LocalePatch>,
  adminLocaleCoveragePatches
)
