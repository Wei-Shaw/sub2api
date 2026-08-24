/**
 * 上游倍率探测（upstream billing probe）资格判定。
 *
 * 该探测请求的是 `/v1/sub2api/billing` —— 一个 sub2api 站点之间的 key 级约定端点，
 * 只有「上游本身也是 sub2api 部署」时才会应答。因此资格被限定为受支持的文本平台
 * 的 API-key 账号：
 *
 *   - 平台必须在 UPSTREAM_BILLING_PROBE_PLATFORMS 内；
 *   - 账号类型必须是 apikey（OAuth / Bedrock 无静态 Key 可出示）。
 *
 * 媒体类平台（fal / atlascloud / apiz / higgsfield）是真实的第三方供应商而非 sub2api 中转，
 * 不会实现该端点，探测只会把账号密钥发到一个必然 404 的路径，故一律不合格。
 *
 * 权威来源是后端 service.IsUpstreamBillingProbeIdentity，
 * 若后端白名单变更需同步修改此处。
 */

/** 允许开启上游倍率探测的平台白名单。 */
export const UPSTREAM_BILLING_PROBE_PLATFORMS = [
  'anthropic',
  'openai',
  'gemini',
  'antigravity',
  'grok'
] as const

/**
 * supportsUpstreamBillingProbe 判断给定平台 + 账号类型是否可参与上游倍率探测。
 * 与后端 IsUpstreamBillingProbeIdentity 语义一一对应。
 */
export function supportsUpstreamBillingProbe(
  platform: string | null | undefined,
  accountType: string | null | undefined
): boolean {
  if (accountType !== 'apikey') return false
  if (!platform) return false
  return (UPSTREAM_BILLING_PROBE_PLATFORMS as readonly string[]).includes(platform)
}
