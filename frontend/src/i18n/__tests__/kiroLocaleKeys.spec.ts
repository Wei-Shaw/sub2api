import { describe, expect, it } from 'vitest'
import en from '@/i18n/locales/en'
import zh from '@/i18n/locales/zh'

function flattenKeys(obj: Record<string, any>, prefix = ''): string[] {
  const keys: string[] = []
  for (const [k, v] of Object.entries(obj)) {
    const fullKey = prefix ? `${prefix}.${k}` : k
    if (typeof v === 'object' && v !== null && !Array.isArray(v)) {
      keys.push(...flattenKeys(v, fullKey))
    } else {
      keys.push(fullKey)
    }
  }
  return keys
}

// Kiro 的凭证表单（KiroCredentialFields.vue）与授权向导（KiroAuthWizard.vue）
// 此前完全没有接入 i18n（100% 硬编码中文），这份清单是接入时从这两个组件
// 加 kiroCredentials.ts、CreateAccountModal.vue/EditAccountModal.vue 的
// KIRO_AUTH_METHOD_OPTIONS 渲染处全量提取的 t() key——新增 Kiro UI 文案时
// 请同步补充这里，防止某个 key 只在一份语言文件里加了、另一份漏掉的情况
// （这类 both-add 场景 vue-tsc 未必总能抓到，取决于对象字面量的写法）。
describe('kiro (Amazon Q Developer / CodeWhisperer) locale key completeness', () => {
  const requiredKeys = [
    'admin.accounts.oauth.kiro.authMethodSocial',
    'admin.accounts.oauth.kiro.authMethodBuilderId',
    'admin.accounts.oauth.kiro.authMethodIdc',
    'admin.accounts.oauth.kiro.authMethodApiKey',
    'admin.accounts.oauth.kiro.fieldRequired',
    'admin.accounts.oauth.kiro.fieldRefreshToken',
    'admin.accounts.oauth.kiro.fieldClientId',
    'admin.accounts.oauth.kiro.fieldClientSecret',
    'admin.accounts.oauth.kiro.fieldIssuerUrl',
    'admin.accounts.oauth.kiro.fieldApiKey',
    'admin.accounts.oauth.kiro.refreshTokenLabel',
    'admin.accounts.oauth.kiro.refreshTokenPlaceholderSocial',
    'admin.accounts.oauth.kiro.refreshTokenPlaceholderBuilderId',
    'admin.accounts.oauth.kiro.refreshTokenPlaceholderIdc',
    'admin.accounts.oauth.kiro.refreshTokenAutoHint',
    'admin.accounts.oauth.kiro.leaveEmptyHint',
    'admin.accounts.oauth.kiro.regionLabel',
    'admin.accounts.oauth.kiro.clientIdLabel',
    'admin.accounts.oauth.kiro.clientSecretLabel',
    'admin.accounts.oauth.kiro.profileArnLabel',
    'admin.accounts.oauth.kiro.profileArnHintBuilderId',
    'admin.accounts.oauth.kiro.profileArnHintIdc',
    'admin.accounts.oauth.kiro.issuerUrlLabel',
    'admin.accounts.oauth.kiro.issuerUrlHint',
    'admin.accounts.oauth.kiro.apiKeyLabel',
    'admin.accounts.oauth.kiro.fakeThinkingTitle',
    'admin.accounts.oauth.kiro.fakeThinkingDesc',
    'admin.accounts.oauth.kiro.headingIdc',
    'admin.accounts.oauth.kiro.headingBuilderId',
    'admin.accounts.oauth.kiro.idleDescIdc',
    'admin.accounts.oauth.kiro.idleDescBuilderId',
    'admin.accounts.oauth.kiro.startButtonLoading',
    'admin.accounts.oauth.kiro.startButtonIdc',
    'admin.accounts.oauth.kiro.startButtonBuilderId',
    'admin.accounts.oauth.kiro.issuerUrlRequiredHint',
    'admin.accounts.oauth.kiro.startedIdcOpenedHint',
    'admin.accounts.oauth.kiro.startedIdcReopenLink',
    'admin.accounts.oauth.kiro.startedIdcInstructions',
    'admin.accounts.oauth.kiro.expiresInSeconds',
    'admin.accounts.oauth.kiro.sessionExpired',
    'admin.accounts.oauth.kiro.submitButtonLoading',
    'admin.accounts.oauth.kiro.submitCallbackUrl',
    'admin.accounts.oauth.kiro.restartButton',
    'admin.accounts.oauth.kiro.userCodeCopied',
    'admin.accounts.oauth.kiro.userCodeCopy',
    'admin.accounts.oauth.kiro.builderIdOpenedHint',
    'admin.accounts.oauth.kiro.builderIdOpenLink',
    'admin.accounts.oauth.kiro.deviceCodeExpiresInSeconds',
    'admin.accounts.oauth.kiro.deviceCodeExpired',
    'admin.accounts.oauth.kiro.reacquireButton',
    'admin.accounts.oauth.kiro.successMessage',
    'admin.accounts.oauth.kiro.reauthorizeButton',
    'admin.accounts.oauth.kiro.errorMissingIssuerUrl',
    'admin.accounts.oauth.kiro.errorGenerateAuthUrlFailed',
    'admin.accounts.oauth.kiro.errorMissingCallbackUrl',
    'admin.accounts.oauth.kiro.errorParseCallbackFailed',
    'admin.accounts.oauth.kiro.errorCompleteAuthFailed',
    'admin.accounts.oauth.kiro.errorGetDeviceCodeFailed',
    'admin.accounts.oauth.kiro.errorDeviceAuthFailed'
  ]

  const enKeys = flattenKeys(en)
  const zhKeys = flattenKeys(zh)

  for (const key of requiredKeys) {
    it(`en locale has ${key}`, () => {
      expect(enKeys).toContain(key)
    })
    it(`zh locale has ${key}`, () => {
      expect(zhKeys).toContain(key)
    })
  }
})
