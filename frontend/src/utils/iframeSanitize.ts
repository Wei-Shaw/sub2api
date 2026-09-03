/**
 * 自定义页面 (CustomPageView) Markdown 正文的 iframe 白名单净化策略。
 *
 * 背景：自定义页面是全站唯一放宽 DOMPurify 白名单（ADD_TAGS: ['iframe']）的地方，
 * 页面作者是管理员，但正文对所有已登录用户可见；若不加约束，作者可以嵌入任意
 * iframe（含 `data:` URI）实施点击劫持 / 应用内钓鱼。本模块保留 iframe 嵌入能力，
 * 但强制：
 *   1. src 必须是绝对 https 地址，且主机命中白名单（精确匹配或点边界子域）；
 *   2. 幸存的 iframe 一律由我们重写 sandbox / referrerpolicy，作者无法自带这些属性；
 *   3. 其余净化行为与此前完全一致（默认 DOMPurify 配置 + 原有 ADD_TAGS / ADD_ATTR）。
 *
 * 所有 hook 都注册在本模块私有的 DOMPurify 实例上，不会污染全局默认实例
 * （AnnouncementBell / ModelPlazaContent / LegalDocumentView 等仍走默认实例）。
 */
import createDOMPurify from 'dompurify'
import type { Config } from 'dompurify'

/**
 * 默认允许被嵌入的主机白名单。
 *
 * 仓库内目前没有“自定义页面实际嵌入了哪些站点”的配置项或文档，因此这里给出的是
 * 常见视频 / 演示嵌入源的保守默认值。命中规则是「主机名完全相等，或以 `.<entry>`
 * 结尾」，所以列出 `youtube.com` 即同时覆盖 `www.youtube.com`；而 `evil-youtube.com`
 * 与 `youtube.com.evil.com` 都不会命中。
 *
 * TODO(运维可配置化)：这份列表最终应由管理员在后台配置，落到公开设置里下发。
 * 需要改动的位置（均不属于本次改动范围）：
 *   - backend: 在系统设置中新增 `custom_page_iframe_hosts`（字符串数组）并在
 *     public settings handler 中输出；
 *   - frontend: `src/types/index.ts` 的 `PublicSettings` 增加同名可选字段；
 *   - frontend: 管理端设置页增加编辑入口。
 * CustomPageView 已经通过 {@link resolveAllowedIframeHosts} 读取
 * `cachedPublicSettings.custom_page_iframe_hosts`，一旦上述字段存在即可自动生效，
 * 无需再改本模块。
 */
export const DEFAULT_ALLOWED_IFRAME_HOSTS: readonly string[] = Object.freeze([
  'youtube.com',
  'youtube-nocookie.com',
  'player.vimeo.com',
  'bilibili.com',
])

/**
 * 强制写入每个幸存 iframe 的 sandbox token。
 *
 * 刻意不包含 `allow-same-origin`：它与 `allow-scripts` 同时出现时，被嵌入页面可以
 * 直接移除自身的 sandbox 属性，等于没有沙箱；不给 `allow-same-origin` 时该 frame 处于
 * 不透明源，拿不到 cookie / localStorage，也无法访问父页面（本站的 access/refresh token
 * 就存在 localStorage 里）。
 * - `allow-scripts`：视频播放器必需。
 * - `allow-popups`：播放器上的“在 YouTube 观看”等外链按钮。
 * - `allow-presentation`：全屏 / 投屏 (Presentation API)。
 * 其余能力（`allow-forms`、`allow-modals`、`allow-downloads`、`allow-top-navigation*`、
 * `allow-popups-to-escape-sandbox`）一律不给，避免被嵌入页劫持顶层导航或诱导下载。
 */
export const IFRAME_SANDBOX_TOKENS = 'allow-scripts allow-popups allow-presentation'

/** 幸存 iframe 上允许保留的作者属性；其余（含作者自带的 sandbox / allow / srcdoc）一律删除。 */
const IFRAME_KEPT_ATTRIBUTES: ReadonlySet<string> = new Set([
  'src',
  'title',
  'width',
  'height',
  'allowfullscreen',
  'frameborder',
])

/** 与改动前保持一致的净化配置：仅放开 iframe 标签与这几个属性（每次调用新建，避免被复用修改）。 */
function buildSanitizeConfig(): Config {
  return {
    ADD_TAGS: ['iframe'],
    ADD_ATTR: ['allowfullscreen', 'frameborder', 'src'],
  }
}

/**
 * 归一化一条主机白名单条目：去空白、去尾点、转小写，并拒绝明显不是主机名的输入
 * （带协议、路径、端口、通配符、用户名等）。返回 null 表示该条目应被忽略。
 */
function normalizeHostEntry(entry: unknown): string | null {
  if (typeof entry !== 'string') return null
  let host = entry.trim().toLowerCase()
  if (!host) return null
  // 允许运维写成 `.example.com` 的形式
  while (host.startsWith('.')) host = host.slice(1)
  while (host.endsWith('.')) host = host.slice(0, -1)
  if (!host) return null
  // 主机名字符集：字母、数字、连字符、点（含 punycode 的 xn-- 前缀）
  if (!/^[a-z0-9-]+(\.[a-z0-9-]+)+$/.test(host)) return null
  return host
}

/**
 * 解析运维配置的主机白名单；为空或全部非法时回落到 {@link DEFAULT_ALLOWED_IFRAME_HOSTS}。
 * 入参故意放宽成 unknown，便于直接喂入后端下发的公开设置字段。
 */
export function resolveAllowedIframeHosts(configured?: unknown): string[] {
  if (!Array.isArray(configured)) return [...DEFAULT_ALLOWED_IFRAME_HOSTS]
  const normalized: string[] = []
  for (const entry of configured) {
    const host = normalizeHostEntry(entry)
    if (host && !normalized.includes(host)) normalized.push(host)
  }
  return normalized.length > 0 ? normalized : [...DEFAULT_ALLOWED_IFRAME_HOSTS]
}

/**
 * 判断 iframe 的 src 是否可以放行。
 *
 * 只接受绝对 https URL；`data:` / `javascript:` / `blob:` / `http:` / 协议相对
 * (`//host/x`) / 相对路径一律拒绝。主机匹配为「完全相等」或「以 `.<entry>` 结尾」，
 * 因此 `evil-youtube.com`、`youtube.com.evil.com` 都不会命中 `youtube.com`。
 * 额外拒绝带用户名/密码的 URL 与非 443 显式端口。
 */
export function isAllowedIframeSrc(
  src: string | null | undefined,
  allowedHosts: readonly string[] = DEFAULT_ALLOWED_IFRAME_HOSTS,
): boolean {
  if (typeof src !== 'string') return false
  const trimmed = src.trim()
  if (!trimmed) return false

  let url: URL
  try {
    // 不传 base：相对地址与协议相对地址在这里直接抛错，符合预期
    url = new URL(trimmed)
  } catch {
    return false
  }

  if (url.protocol !== 'https:') return false
  if (url.username || url.password) return false
  if (url.port && url.port !== '443') return false

  const hostname = url.hostname.toLowerCase().replace(/\.$/, '')
  if (!hostname) return false

  return allowedHosts.some((raw) => {
    const host = normalizeHostEntry(raw)
    if (!host) return false
    return hostname === host || hostname.endsWith(`.${host}`)
  })
}

/**
 * 私有 DOMPurify 实例：hook 是实例级的，注册在这里就不会影响
 * `import DOMPurify from 'dompurify'` 得到的全局默认实例。
 */
let purifier: ReturnType<typeof createDOMPurify> | null = null
/** 当前这次 sanitize 生效的主机白名单，由 sanitizeCustomPageHtml 在调用前设置。 */
let activeAllowedHosts: readonly string[] = DEFAULT_ALLOWED_IFRAME_HOSTS

function getPurifier(): ReturnType<typeof createDOMPurify> {
  if (purifier) return purifier
  // 不传 root，DOMPurify 默认取全局 window，但返回的是独立实例
  const instance = createDOMPurify()
  instance.addHook('afterSanitizeAttributes', (node) => {
    if (!node || typeof (node as Element).getAttribute !== 'function') return
    const el = node as Element
    if (el.nodeName.toLowerCase() !== 'iframe') return

    if (!isAllowedIframeSrc(el.getAttribute('src'), activeAllowedHosts)) {
      el.parentNode?.removeChild(el)
      return
    }

    // 删掉作者提供的一切非白名单属性（尤其是 sandbox / allow / srcdoc / name / on*）
    for (const attr of Array.from(el.attributes)) {
      if (!IFRAME_KEPT_ATTRIBUTES.has(attr.name.toLowerCase())) {
        el.removeAttribute(attr.name)
      }
    }

    el.setAttribute('sandbox', IFRAME_SANDBOX_TOKENS)
    el.setAttribute('referrerpolicy', 'no-referrer')
    el.setAttribute('loading', 'lazy')
  })
  purifier = instance
  return instance
}

export interface SanitizeCustomPageOptions {
  /** 运维配置的主机白名单；省略或为空时使用默认列表。 */
  allowedIframeHosts?: readonly string[]
}

/**
 * 净化自定义页面 Markdown 渲染出的 HTML：默认 DOMPurify 规则 + 受限的 iframe 嵌入。
 */
export function sanitizeCustomPageHtml(
  html: string,
  options: SanitizeCustomPageOptions = {},
): string {
  if (!html) return ''
  const hosts = options.allowedIframeHosts?.length
    ? options.allowedIframeHosts
    : DEFAULT_ALLOWED_IFRAME_HOSTS
  activeAllowedHosts = hosts
  try {
    return getPurifier().sanitize(html, buildSanitizeConfig()) as string
  } finally {
    activeAllowedHosts = DEFAULT_ALLOWED_IFRAME_HOSTS
  }
}
