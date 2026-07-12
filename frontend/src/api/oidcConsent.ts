/**
 * OIDC Provider 同意页 API。
 *
 * 注意：这些端点挂在根路径 `/oidc/consent`（无 `/api/v1` 前缀），且依赖 HttpOnly
 * 的 SSO cookie (`sub2api_sso`) 做登录态校验，因此使用独立的 axios 实例并开启
 * `withCredentials`。后端返回的是原始 JSON（非 `{code,message,data}` 信封）。
 */

import axios, { AxiosError } from 'axios'
import { getLocale } from '@/i18n'

const consentClient = axios.create({
  baseURL: '/',
  withCredentials: true,
  timeout: 30000,
  headers: {
    'Content-Type': 'application/json'
  }
})

consentClient.interceptors.request.use((config) => {
  if (config.headers) {
    config.headers['Accept-Language'] = getLocale()
  }
  return config
})

export interface OidcConsentScope {
  scope: string
  sensitive: boolean
}

export interface OidcConsentInfo {
  client_id: string
  client_name: string
  scopes: OidcConsentScope[]
}

export interface OidcConsentDecisionResult {
  redirect: string
}

/** OAuth 风格错误：`{ error, error_description }`。 */
export interface OidcConsentError {
  status: number
  error: string
  error_description?: string
}

function normalizeError(err: unknown): OidcConsentError {
  const axiosErr = err as AxiosError<{ error?: string; error_description?: string }>
  const status = axiosErr.response?.status ?? 0
  const data = axiosErr.response?.data
  return {
    status,
    error: data?.error || 'request_failed',
    error_description: data?.error_description
  }
}

/** 拉取当前同意请求的展示信息。 */
export async function getConsentInfo(consent: string): Promise<OidcConsentInfo> {
  try {
    const { data } = await consentClient.get<OidcConsentInfo>('/oidc/consent', {
      params: { consent }
    })
    return data
  } catch (err) {
    throw normalizeError(err)
  }
}

/** 提交用户决策（allow/deny），返回后端计算好的回跳地址。 */
export async function submitConsentDecision(
  consent: string,
  action: 'allow' | 'deny'
): Promise<OidcConsentDecisionResult> {
  try {
    const { data } = await consentClient.post<OidcConsentDecisionResult>('/oidc/consent', {
      consent,
      action
    })
    return data
  } catch (err) {
    throw normalizeError(err)
  }
}
