/**
 * 登录 / OAuth 回调后的跳转路径清洗。
 *
 * 该实现从各 OAuth 回调页面（OAuthCallbackView / WechatCallbackView /
 * OidcCallbackView / DingTalkCallbackView / LinuxDoCallbackView /
 * DingTalkEmailCompletionView）中抽取并统一，前五条规则与原有本地副本逐字节一致。
 */
export const DEFAULT_REDIRECT_PATH = '/dashboard'

/**
 * URL 解析器在解析前会直接删除的字符：制表符 U+0009、换行 U+000A、回车 U+000D。
 * 因此 `/<TAB>/evil.com` 会被浏览器还原成 `//evil.com`。
 */
const URL_STRIPPED_CHARS = /[\t\n\r]/g

/**
 * 把输入还原成「浏览器实际会看到的样子」：
 * 1. 删除 URL 解析器会忽略的 TAB / LF / CR；
 * 2. 对 http(s) 这类 special scheme，反斜杠等价于正斜杠，统一成正斜杠。
 */
function normalizeForAuthorityCheck(path: string): string {
  return path.replace(URL_STRIPPED_CHARS, '').replace(/\\/g, '/')
}

/**
 * 清洗跳转路径，防止开放重定向（open redirect）。
 *
 * 拒绝规则（任一命中即回落到 `/dashboard`）：
 * 1. 空值：undefined / null / 空字符串
 * 2. 不以 `/` 开头：相对路径、`javascript:` 等伪协议、绝对 URL、裸 `\evil.com`
 * 3. 以 `//` 开头：协议相对 URL，例如 `//evil.com`
 * 4. 包含 `://`：绝对 URL，例如 `https://evil.com`
 * 5. 包含 `\n` 或 `\r`：CRLF 注入
 * 6. 归一化后以 `//` 开头：覆盖反斜杠与 TAB 变体，例如 `/\evil.com`、
 *    `/\/evil.com`、`/\\evil.com`、`/<TAB>/evil.com`
 *
 * 规则 6 只检查「权限段（authority）位置」，即归一化后的前两个字符。
 * 路径中部的反斜杠（如 `/docs/a\b`）不拦截：它归一化成 `/docs/a/b`，
 * 仍然是同源路径，拦截它只会误伤合法链接。
 *
 * @param path 待清洗的路径，通常来自 `route.query.redirect` 或后端返回的 redirect
 * @returns 可安全用于 router.push / router.replace 的站内路径
 */
export function sanitizeRedirectPath(path: string | null | undefined): string {
  if (!path) return DEFAULT_REDIRECT_PATH
  if (!path.startsWith('/')) return DEFAULT_REDIRECT_PATH
  if (path.startsWith('//')) return DEFAULT_REDIRECT_PATH
  if (path.includes('://')) return DEFAULT_REDIRECT_PATH
  if (path.includes('\n') || path.includes('\r')) return DEFAULT_REDIRECT_PATH
  if (normalizeForAuthorityCheck(path).startsWith('//')) return DEFAULT_REDIRECT_PATH
  return path
}
