/**
 * Kiro 账号凭证构造。
 *
 * 字段名必须与后端 credentials schema 严格一致
 * （backend/internal/service/kiro_credentials.go）——
 * 命名漂移会让账号建得出来但转发时取不到凭证，症状是 401 而非表单错误。
 */

import { i18n } from '@/i18n'

const { t } = i18n.global

export type KiroAuthMethod = 'social' | 'builder_id' | 'idc' | 'api_key'

// labelKey 而不是直接存 label 文本——组件里用 t(opt.labelKey) 渲染，
// 这样切换语言时模板重新渲染就能拿到当前语言的文本，不需要把这个数组
// 本身做成响应式的 computed。
export const KIRO_AUTH_METHOD_OPTIONS: readonly { value: KiroAuthMethod; labelKey: string }[] = [
  { value: 'social', labelKey: 'admin.accounts.oauth.kiro.authMethodSocial' },
  { value: 'builder_id', labelKey: 'admin.accounts.oauth.kiro.authMethodBuilderId' },
  { value: 'idc', labelKey: 'admin.accounts.oauth.kiro.authMethodIdc' },
  { value: 'api_key', labelKey: 'admin.accounts.oauth.kiro.authMethodApiKey' }
] as const

export interface KiroCredentialForm {
  authMethod: KiroAuthMethod
  refreshToken: string
  accessToken: string
  clientId: string
  clientSecret: string
  issuerUrl: string
  region: string
  profileArn: string
  apiKey: string
  fakeThinking: boolean
}

const trim = (v: string | undefined): string => (v ?? '').trim()

/** 按接入方式返回必填字段名，供表单标星与提交前校验共用。 */
export function kiroRequiredFields(method: KiroAuthMethod): string[] {
  switch (method) {
    case 'api_key':
      return ['apiKey']
    case 'idc':
      return ['refreshToken', 'clientId', 'clientSecret', 'issuerUrl']
    case 'builder_id':
      return ['refreshToken', 'clientId', 'clientSecret']
    default:
      return ['refreshToken']
  }
}

/** 校验表单，返回错误提示；通过时返回 null。 */
export function validateKiroCredentials(form: KiroCredentialForm): string | null {
  const labelKeys: Record<string, string> = {
    refreshToken: 'admin.accounts.oauth.kiro.fieldRefreshToken',
    clientId: 'admin.accounts.oauth.kiro.fieldClientId',
    clientSecret: 'admin.accounts.oauth.kiro.fieldClientSecret',
    issuerUrl: 'admin.accounts.oauth.kiro.fieldIssuerUrl',
    apiKey: 'admin.accounts.oauth.kiro.fieldApiKey'
  }

  for (const field of kiroRequiredFields(form.authMethod)) {
    if (!trim((form as unknown as Record<string, string>)[field])) {
      const fieldLabel = labelKeys[field] ? t(labelKeys[field]) : field
      return t('admin.accounts.oauth.kiro.fieldRequired', { field: fieldLabel })
    }
  }
  return null
}

/**
 * 构造提交给后端的 credentials。
 *
 * 只提交当前接入方式实际需要的字段，避免残留字段让后端的 auth_method
 * 分派产生歧义。machine_id 一律不提交 —— 它由后端在建号时固化，
 * 前端插手会破坏设备指纹的稳定性。
 */
export function buildKiroCredentials(form: KiroCredentialForm): Record<string, unknown> {
  const creds: Record<string, unknown> = { auth_method: form.authMethod }

  const region = trim(form.region)
  if (region) creds.region = region

  const profileArn = trim(form.profileArn)
  if (profileArn && form.authMethod !== 'api_key') creds.profile_arn = profileArn

  if (form.authMethod === 'api_key') {
    creds.api_key = trim(form.apiKey)
  } else {
    creds.refresh_token = trim(form.refreshToken)

    const accessToken = trim(form.accessToken)
    if (accessToken) creds.access_token = accessToken

    if (form.authMethod === 'idc' || form.authMethod === 'builder_id') {
      creds.client_id = trim(form.clientId)
      creds.client_secret = trim(form.clientSecret)
    }
    if (form.authMethod === 'idc') {
      creds.issuer_url = trim(form.issuerUrl)
    }
  }

  // 假思考默认开启（后端 KiroFakeThinking() 未显式配置时按 true 处理）——
  // 必须显式提交这个布尔值，不能只在 true 时才带上，否则勾选框被取消后
  // 提交的 payload 里这个键直接消失，后端会按"未配置"退回默认开启，
  // 关不掉。
  creds.fake_thinking = form.fakeThinking

  return creds
}
