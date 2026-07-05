import { computed, onBeforeUnmount, onMounted, ref, type ComputedRef } from 'vue'

/**
 * 自定义菜单红点 dismiss 的一次性方案。
 *
 * 与 `useRechargePromoDot` 共享同一个哲学：
 *   - localStorage key = `custom-menu-seen:<userId>:<version>`，用户看过即写入；
 *   - `version` 由后端针对 { enabled, items 规范化 JSON } 派生，no-op 保存不会
 *     改 version，因此不会打扰已 dismiss 的用户；
 *   - 跨 tab 通过 `storage` 事件同步；同 tab 通过模块级共享 tick 广播。
 *
 * 与充值红点的差异：
 *   - 全局粒度：所有自定义菜单项共享一个 dismiss key，一次点击即可清空所有红点；
 *   - 三处 dismiss 触发点：sidebar / drawer 点击 + `/custom/:id` 挂载。
 */
export interface UseCustomMenuRedDotOptions {
  /** 当前登录用户 id；未登录返回 null。 */
  userId: ComputedRef<number | null>
  /** 是否开启红点提醒（admin 显式开关）。 */
  enabled: ComputedRef<boolean>
  /** 后端派生的短 hash version；no-op save 时保持不变。 */
  version: ComputedRef<string | undefined>
}

export interface UseCustomMenuRedDotReturn {
  shouldShow: ComputedRef<boolean>
  dismiss: () => void
}

const SEEN_PREFIX = 'custom-menu-seen'

/** 计算 storage key，缺少必要参数时返回 null（调用方据此跳过）。 */
function buildKey(userId: number | null, version: string | undefined): string | null {
  if (userId == null || !version) return null
  return `${SEEN_PREFIX}:${userId}:${version}`
}

/**
 * 模块级共享 tick：同 tab 内多个实例（sidebar 与 CustomPageView 同时挂载）
 * 需要在任一实例 dismiss 后立即刷新其他实例的 shouldShow。
 * 参考 useRechargePromoDot 的相同处理。
 */
const sharedDismissTick = ref(0)

function bumpSharedTick(): void {
  sharedDismissTick.value += 1
}

export function useCustomMenuRedDot(options: UseCustomMenuRedDotOptions): UseCustomMenuRedDotReturn {
  const { userId, enabled, version } = options

  const storageKey = computed(() => buildKey(userId.value, version.value))

  const shouldShow = computed<boolean>(() => {
    void sharedDismissTick.value // 触发依赖：dismiss/跨 tab 变化都会改它
    if (!enabled.value) return false
    const key = storageKey.value
    if (!key) return false
    if (typeof window === 'undefined') return false
    try {
      return window.localStorage.getItem(key) == null
    } catch {
      // localStorage 在隐私模式下可能抛错；按“已看过”处理避免反复出红点。
      return false
    }
  })

  function dismiss(): void {
    const key = storageKey.value
    if (!key) return
    if (typeof window === 'undefined') return
    try {
      window.localStorage.setItem(key, '1')
    } catch {
      // 同上：localStorage 不可写时静默忽略。
    }
    bumpSharedTick()
  }

  function onStorage(event: StorageEvent): void {
    // storage 事件只在“其他” tab 触发：当其他 tab 写入任意
    // `custom-menu-seen:*` key（或 storage 被清空）时，通知本 tab 重新计算。
    if (
      event.key === null ||
      (typeof event.key === 'string' && event.key.startsWith(`${SEEN_PREFIX}:`))
    ) {
      bumpSharedTick()
    }
  }

  onMounted(() => {
    if (typeof window !== 'undefined') {
      window.addEventListener('storage', onStorage)
    }
  })

  onBeforeUnmount(() => {
    if (typeof window !== 'undefined') {
      window.removeEventListener('storage', onStorage)
    }
  })

  return { shouldShow, dismiss }
}
