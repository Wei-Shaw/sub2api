/**
 * Plugin-local page-size persistence helpers.
 *
 * 替代 ChannelsView 的 `'table.pageSize'` 与 ChannelMonitorView 的
 * `'channelMonitor.pageSize'` 两份漂移的 localStorage key. 统一 key 命名空间,
 * 防止后续 view 各自再挑一份字面量.
 *
 * Key 格式: `channel-mgmt.pageSize.<scope>` (e.g. `channel-mgmt.pageSize.channels`).
 * 浏览器/隐身窗 SSR 等无 localStorage 场景静默忽略, 保留 fallback.
 *
 * 旧 key 兼容: T19 Fix A 引入 `LEGACY_KEYS` 一次性迁移逻辑 — 当新 key 不存在但
 * 旧 key 存在时, 读取旧值后写入新 key 并删除旧 key, 避免现网用户的 pageSize
 * 偏好在 T10 重命名后丢失.
 */

const KEY_PREFIX = 'channel-mgmt.pageSize'
const DEFAULT_PAGE_SIZE = 20
const VALID_PAGE_SIZES = [10, 20, 50, 100] as const

/**
 * 旧 localStorage key 映射表 (scope -> 旧 key).
 * 仅用于一次性迁移; 完成迁移后旧 key 会被 `localStorage.removeItem` 清掉.
 *
 * - `channels`: ChannelsView 历史 key (T10 之前)
 * - `channelMonitor`: ChannelMonitorView 历史 key (T10 之前)
 */
const LEGACY_KEYS: Record<string, string> = {
  channels: 'table.pageSize',
  channelMonitor: 'channelMonitor.pageSize',
}

export type PageSize = (typeof VALID_PAGE_SIZES)[number]

function storageKey(scope: string): string {
  return `${KEY_PREFIX}.${scope}`
}

function isValid(n: number): boolean {
  return Number.isFinite(n) && n > 0
}

/** 解析 localStorage 字符串为合法正整数, 否则返回 null. */
function parseValid(raw: string): number | null {
  const n = Number.parseInt(raw, 10)
  return isValid(n) ? n : null
}

/**
 * 读取已持久化的 pageSize. 不在 VALID_PAGE_SIZES 内的值仍会被接受
 * (历史用户配置可能写过 25 等非标准值, 不强制 reset), 仅过滤非法值.
 *
 * 优先级:
 *   1. 新 key (`channel-mgmt.pageSize.<scope>`)
 *   2. 旧 key (`LEGACY_KEYS[scope]`, 若存在则一次性迁移到新 key)
 *   3. fallback 默认值
 */
export function loadPageSize(scope: string, fallback: number = DEFAULT_PAGE_SIZE): number {
  try {
    // 1) 优先读新 key
    const raw = window.localStorage.getItem(storageKey(scope))
    if (raw) {
      const parsed = parseValid(raw)
      if (parsed != null) return parsed
    }
    // 2) fallback 旧 key (一次性迁移)
    const legacy = LEGACY_KEYS[scope]
    if (legacy) {
      const legacyRaw = window.localStorage.getItem(legacy)
      if (legacyRaw) {
        const parsed = parseValid(legacyRaw)
        if (parsed != null) {
          // 写入新 key + 删除旧 key 完成迁移
          savePageSize(scope, parsed)
          try {
            window.localStorage.removeItem(legacy)
          } catch {
            // ignore
          }
          return parsed
        }
      }
    }
    return fallback
  } catch {
    // SSR / sandboxed iframe — ignore
    return fallback
  }
}

/** 写入 pageSize. 非法值不写, 防止下次加载读到坏值. */
export function savePageSize(scope: string, size: number): void {
  if (!isValid(size)) return
  try {
    window.localStorage.setItem(storageKey(scope), String(size))
  } catch {
    // ignore
  }
}

export { DEFAULT_PAGE_SIZE, VALID_PAGE_SIZES }
