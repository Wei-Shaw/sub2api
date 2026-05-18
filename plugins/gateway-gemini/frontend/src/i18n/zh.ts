/**
 * Chinese i18n messages for the Gemini gateway plugin.
 */
export default {
  admin: {
    accounts: {
      gemini: {
        helpButton: '使用帮助',
        helpDialog: {
          title: 'Gemini 使用指南',
          apiKeySection: 'API Key 相关链接'
        },
        modelPassthrough: 'Gemini 直接转发模型',
        modelPassthroughDesc: '所有模型请求将直接转发至 Gemini API，不进行模型限制或映射。',
        baseUrlHint: '留空使用官方 Gemini API',
        apiKeyHint: '您的 Gemini API Key（以 AIza 开头）',
        tier: {
          label: '账号等级',
          hint: '提示：系统会优先尝试自动识别账号等级；若自动识别不可用或失败，则使用你选择的等级作为回退（本地模拟配额）。',
          aiStudioHint:
            'AI Studio 的配额是按模型分别限流（Pro/Flash 独立）。若已绑卡（按量付费），请选 Pay-as-you-go。',
          googleOne: {
            free: 'Google One Free',
            pro: 'Google One Pro',
            ultra: 'Google One Ultra'
          },
          gcp: {
            standard: 'GCP Standard',
            enterprise: 'GCP Enterprise'
          },
          aiStudio: {
            free: 'Google AI Free',
            paid: 'Google AI Pay-as-you-go'
          }
        },
        accountType: {
          oauthTitle: 'OAuth 授权（Gemini）',
          oauthDesc: '使用 Google 账号授权，并选择 OAuth 子类型。',
          apiKeyTitle: 'API 密钥（AI Studio）',
          apiKeyDesc: '最快接入方式，使用 AIza API Key。',
          apiKeyNote: '适合轻量测试。免费层限流严格，数据可能用于训练。',
          apiKeyLink: '获取 API Key',
          quotaLink: '配额说明'
        },
        oauthType: {
          googleOneDesc: '个人账号，享受 Google One 订阅配额',
          codeAssistDesc: '企业级，需要 GCP 项目',
          advancedToggle: '高级选项',
          builtInTitle: '内置授权（Gemini CLI / Code Assist）',
          builtInDesc: '使用 Google 内置客户端 ID，无需管理员配置。',
          builtInRequirement: '需要 GCP 项目并填写 Project ID。',
          gcpProjectLink: '创建项目',
          customTitle: '自定义授权（AI Studio OAuth）',
          customDesc: '使用管理员预设的 OAuth 客户端，适合组织管理。',
          customRequirement: '需管理员配置 Client ID 并加入测试用户白名单。',
          badges: {
            recommended: '推荐',
            highConcurrency: '高并发',
            noAdmin: '无需管理员配置',
            orgManaged: '组织管理',
            adminRequired: '需要管理员'
          }
        },
        rateLimit: {
          ok: '未限流',
          unlimited: '无限流',
          limited: '限流 {time}',
          now: '现在'
        },
        test: {
          imagePromptLabel: '图像提示词',
          imagePromptPlaceholder: '描述你想要生成的图像...',
          imagePromptHint: 'Imagen 模型根据文本描述生成图像。',
          imagePromptDefault: '日落时分的宁静山景，伴有倒映的湖泊'
        }
      },
      oauth: {
        gemini: {
          oauthTypeLabel: 'OAuth 类型',
          aiStudioNotConfiguredShort: '未配置',
          aiStudioNotConfiguredTip:
            'AI Studio OAuth 未配置：请先设置 GEMINI_OAUTH_CLIENT_ID / GEMINI_OAUTH_CLIENT_SECRET，并在 Google OAuth Client 添加 Redirect URI：http://localhost:1455/auth/callback（Consent Screen scopes 需包含 https://www.googleapis.com/auth/generative-language.retriever）',
          failedToGenerateUrl: '生成 Gemini 授权链接失败',
          missingExchangeParams: '缺少 code / session_id / state',
          failedToExchangeCode: 'Gemini 授权码兑换失败',
          missingProjectId:
            'GCP Project ID 获取失败：您的 Google 账号未关联有效的 GCP 项目。请前往 Google Cloud Console 激活 GCP 并绑定信用卡，或在授权时手动填写 Project ID。'
        }
      }
    }
  }
}
