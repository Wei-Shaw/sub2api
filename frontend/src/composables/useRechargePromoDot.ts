import { computed, onBeforeUnmount, onMounted, ref, type ComputedRef } from 'vue'
import type { RechargePromo } from '@/types/payment'

/**
 * 充值赠送活动红点 dismiss 的整体一次方案。
 *
 * 设计取舍：
 *   - 红点用 localStorage 落 `${userId}:${version}` 一份 key 而不是每天刷新；
 *     用户看过即代表"知道这个活动"，活动版本变化（version 变了）时再次出红点。
 *   - 跨标签页通过监听 `storage` 事件保持同步，这样在另一个 tab dismiss 后
 *     当前 tab 不会还顶着红点。
 *
 * 与服务端的耦合点：`promo.version` 由后端针对规范化 JSON 计算，
 *   后端 no-op save 不会改 version，前端因此不会刷红点 → 这是“不刷新”需求。
 */
export interface UseRechargePromoDotOptions {
  /** 当前登录用户 id；未登录返回 null。 */
  userId: ComputedRef<number | null>
  /** 后端下发的活动配置，未启用 / 不在窗口内时为 null。 */
  promo: ComputedRef<RechargePromo | null | undefined>
}

export interface UseRechargePromoDotReturn {
  shouldShow: ComputedRef<boolean>
  dismiss: () => void
}

const SEEN_PREFIX = 'recharge-promo-seen'

/** 计算 storage key，缺少必要参数时返回 null（调用方据此跳过）。 */
function buildKey(userId: number | null, version: string | undefined): string | null {
  if (userId == null || !version) return null
  return `${SEEN_PREFIX}:${userId}:${version}`
}

/**
 * 模块级共享 tick：同一个 SPA 内任意实例 dismiss 后，
 * 其他实例（如 sidebar 与 PaymentView 同时挂载时）也能立刻看到状态变化。
 *
 * 仅靠原生 `storage` 事件不够：浏览器规范明确指出该事件只在“其他”同源 tab 触发，
 * 同 tab 内 setItem 的写入不会通知本 tab 的监听器。所以用一个 module-scoped ref
 * 作为额外的同 tab 广播通道；跨 tab 仍走 storage 事件，最终也写到这个 tick 上。
 */
const sharedDismissTick = ref(0)

function bumpSharedTick(): void {
  sharedDismissTick.value += 1
}

export function useRechargePromoDot(options: UseRechargePromoDotOptions): UseRechargePromoDotReturn {
  const { userId, promo } = options

  const storageKey = computed(() => buildKey(userId.value, promo.value?.version))

  const shouldShow = computed<boolean>(() => {
    void sharedDismissTick.value // 触发依赖：dismiss/跨 tab 变化都会改它
    const p = promo.value
    if (!p || !p.enabled || !p.version) return false
    const key = storageKey.value
    if (!key) return false
    if (typeof window === 'undefined') return false
    try {
      return window.localStorage.getItem(key) == null
    } catch {
      // localStorage 在隐私模式下可能抛错；按"已看过"处理避免反复出红点
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
      // 同上：localStorage 不可写时静默忽略，红点会在下次刷新前继续显示
    }
    // 通知本 tab 内的所有实例（含 sidebar 红点）立刻刷新。
    bumpSharedTick()
  }

  function onStorage(event: StorageEvent): void {
    // storage 事件只在“其他” tab 触发：当其他 tab dismiss 了任何 promo key
    // （或整个 storage 被清空）时，让本 tab 的所有实例重算。
    if (event.key === null || (typeof event.key === 'string' && event.key.startsWith(`${SEEN_PREFIX}:`))) {
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
