import { computed, onBeforeUnmount, onMounted, ref, type ComputedRef } from 'vue'

/**
 * 自定义菜单红点：**每项独立** dismiss 的一次性方案。
 *
 * 与 `useRechargePromoDot` 相同哲学：
 *   - localStorage key = `custom-menu-seen:<userId>:<itemId>:<version>`；
 *   - `version` 由后端针对 items 展示字段规范化 JSON 派生，no-op 保存不会
 *     改 version，因此不会打扰已 dismiss 的用户；
 *   - 跨 tab 通过 `storage` 事件同步；同 tab 通过模块级共享 tick 广播。
 *
 * 每项独立差异：
 *   - 每个自定义菜单项一个独立 dismiss key，一次点击只清空该项的红点，其他项照旧；
 *   - `enabled` 由 item.show_red_dot 决定——admin 未勾选的项即便 version 变了也不会亮；
 *   - 使用者：sidebar / drawer / CustomPageView。
 *
 * 使用说明：
 *   - `useCustomMenuRedDot(...)` 是**响应式 composable**，遵循 Vue 规则要求，只在
 *     setup 顶层且以稳定次数调用（如 CustomPageView 单例场景）；
 *   - 在需要**动态数量**（sidebar 里 v-for items）的场景，改用下面的"共享注册器"
 *     `useCustomMenuRedDotRegistry` + 纯函数 `isCustomMenuDotVisibleFor` /
 *     `dismissCustomMenuDotFor`——只在容器组件中挂一次生命周期，
 *     items 的红点由普通闭包按 itemId 查询/dismiss。
 */

const SEEN_PREFIX = 'custom-menu-seen'

function buildKey(
  userId: number | null,
  itemId: string | undefined,
  version: string | undefined,
): string | null {
  if (userId == null || !version) return null
  const trimmedId = (itemId ?? '').trim()
  if (!trimmedId) return null
  return `${SEEN_PREFIX}:${userId}:${trimmedId}:${version}`
}

// 模块级共享 tick：任一实例 dismiss 后立即刷新其他实例的 shouldShow（同 tab）。
export const sharedDismissTick = ref(0)

function bumpSharedTick(): void {
  sharedDismissTick.value += 1
}

// 模块级 storage 事件引用计数 —— 只要有 ≥1 个 registry/composable 存活就注册一次
// 全局 storage 监听。销毁归零时注销，避免零监听时残留 listener 泄漏。
let storageListenerRefCount = 0

function onStorageEvent(event: StorageEvent): void {
  if (
    event.key === null ||
    (typeof event.key === 'string' && event.key.startsWith(`${SEEN_PREFIX}:`))
  ) {
    bumpSharedTick()
  }
}

function acquireStorageListener(): void {
  if (typeof window === 'undefined') return
  storageListenerRefCount += 1
  if (storageListenerRefCount === 1) {
    window.addEventListener('storage', onStorageEvent)
  }
}

function releaseStorageListener(): void {
  if (typeof window === 'undefined') return
  storageListenerRefCount = Math.max(0, storageListenerRefCount - 1)
  if (storageListenerRefCount === 0) {
    window.removeEventListener('storage', onStorageEvent)
  }
}

/**
 * 纯函数：查询给定 (userId, itemId, version) 的红点是否应展示。
 * 调用者需要**自行**订阅 `sharedDismissTick.value` 以让 Vue 建立依赖。
 * 通常配合 `computed(() => ...)` 使用，参见 sidebar 中的 `flagCustomMenuDot(itemId)`。
 */
export function isCustomMenuDotVisibleFor(
  userId: number | null,
  itemId: string | undefined,
  version: string | undefined,
  enabled: boolean,
): boolean {
  if (!enabled) return false
  const key = buildKey(userId, itemId, version)
  if (!key) return false
  if (typeof window === 'undefined') return false
  try {
    return window.localStorage.getItem(key) == null
  } catch {
    return false
  }
}

/** 纯函数：dismiss 单个 (userId, itemId, version) 的红点。 */
export function dismissCustomMenuDotFor(
  userId: number | null,
  itemId: string | undefined,
  version: string | undefined,
): void {
  const key = buildKey(userId, itemId, version)
  if (!key) return
  if (typeof window === 'undefined') return
  try {
    window.localStorage.setItem(key, '1')
  } catch {
    // localStorage 不可写时静默忽略
  }
  bumpSharedTick()
}

export interface UseCustomMenuRedDotRegistryReturn {
  /** 依赖标记：读取此 ref 建立 Vue 响应式依赖，任一 dismiss 都会自增。 */
  tick: ComputedRef<number>
}

/**
 * 在容器组件的 setup 顶层调用一次，负责挂 storage 事件监听 + 归还
 * 响应式 tick 供 v-for 内的红点计算依赖。
 */
export function useCustomMenuRedDotRegistry(): UseCustomMenuRedDotRegistryReturn {
  onMounted(() => acquireStorageListener())
  onBeforeUnmount(() => releaseStorageListener())
  return {
    tick: computed(() => sharedDismissTick.value),
  }
}

export interface UseCustomMenuRedDotOptions {
  userId: ComputedRef<number | null>
  enabled: ComputedRef<boolean>
  version: ComputedRef<string | undefined>
  itemId: ComputedRef<string | undefined>
}

export interface UseCustomMenuRedDotReturn {
  shouldShow: ComputedRef<boolean>
  dismiss: () => void
}

/**
 * 单实例响应式 composable：适用于 CustomPageView 等 setup 稳定单次调用场景。
 * 若要在 v-for 中按 items 动态展示红点，请改用 `useCustomMenuRedDotRegistry`
 * + `isCustomMenuDotVisibleFor` / `dismissCustomMenuDotFor` 组合。
 */
export function useCustomMenuRedDot(options: UseCustomMenuRedDotOptions): UseCustomMenuRedDotReturn {
  const { userId, enabled, version, itemId } = options

  onMounted(() => acquireStorageListener())
  onBeforeUnmount(() => releaseStorageListener())

  const shouldShow = computed<boolean>(() => {
    void sharedDismissTick.value // 触发依赖
    return isCustomMenuDotVisibleFor(userId.value, itemId.value, version.value, enabled.value)
  })

  function dismiss(): void {
    dismissCustomMenuDotFor(userId.value, itemId.value, version.value)
  }

  return { shouldShow, dismiss }
}
