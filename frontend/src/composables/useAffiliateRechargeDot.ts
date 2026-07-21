import { computed, ref, watch } from 'vue'

import { useAuthStore } from '@/stores/auth'
import { useInboxStore } from '@/stores/inbox'
import {
  latestRechargeSeq,
  sumUnreadRebate,
  unreadRebateByInvitee,
  unreadRechargeInviteeIDs,
} from '@/components/common/affiliateRechargeInbox'

/**
 * 邀请返利"新充值红点"的共享状态。
 *
 * 侧边栏「邀请返利」菜单项与 AffiliateView 页面内的置顶/红箭头，都需要同一套"未读"判定，
 * 并且要在"进入返利页查看后"一起熄灭。为此把"已查看水位"抽成模块级共享的响应式 ref：
 *   - 未读 = inbox 里 namespace=affiliate_recharge 且 seq > 已查看水位 的消息；
 *   - markSeen() 推进水位（持久化到 localStorage + 推进 inbox 累积 ack），任一处调用都会
 *     让所有引用此 composable 的组件响应式熄灭红点。
 *
 * 为什么不用 inbox 的 localAckSeq：inbox 收到消息后 ~300ms 会自动 ack 推进 localAckSeq
 * （那是"投递/接收水位"，非"用户已查看"），用它判红点会瞬间清空。
 */
const STORAGE_PREFIX = 'affiliate_recharge_seen:'

// 模块级共享：所有组件复用同一水位 ref，任一处推进都会同步响应。
const seenSeq = ref(0)
let loadedKey = ''
let storageBound = false

function storageKey(uid: string): string {
  return `${STORAGE_PREFIX}${uid}`
}

function readSeen(key: string): number {
  try {
    return parseInt(localStorage.getItem(key) || '0', 10) || 0
  } catch {
    return 0
  }
}

function ensureLoaded(uid: string): void {
  const key = storageKey(uid)
  if (loadedKey !== key) {
    loadedKey = key
    seenSeq.value = readSeen(key)
  }
  if (!storageBound && typeof window !== 'undefined') {
    storageBound = true
    // 跨标签页：其他 tab 查看返利页推进水位后，本 tab 红点同步熄灭。
    window.addEventListener('storage', (e) => {
      if (e.key && e.key === loadedKey) {
        seenSeq.value = readSeen(loadedKey)
      }
    })
  }
}

export function useAffiliateRechargeDot() {
  const authStore = useAuthStore()
  const inboxStore = useInboxStore()

  const uid = computed(() => String(authStore.user?.id ?? 'anon'))
  // 登录用户变化时按 uid 重载水位（幂等）。
  watch(uid, (v) => ensureLoaded(v), { immediate: true })

  /** 有新充值(未读)的被邀请人 id 集合；随 inbox 消息与已查看水位响应式更新。 */
  const unreadInviteeIDs = computed(() =>
    unreadRechargeInviteeIDs(inboxStore.messages, seenSeq.value),
  )
  /** 是否存在任一未读下线充值（用于菜单红点）。 */
  const hasUnread = computed(() => unreadInviteeIDs.value.size > 0)
  /** 最新一条 affiliate_recharge 消息的 seq；用于驱动返利明细的即时刷新（watch 其变化）。 */
  const latestSeq = computed(() => latestRechargeSeq(inboxStore.messages))

  /**
   * 未读充值通知对应的返利总额（用于红字"本次新增返利"）。直接来自消息 payload，
   * 因此新开网页 catchup 到未读消息时也能正确展示。ratePercent 用于旧消息(无 rebate 字段)兜底。
   */
  function unreadRebateTotal(ratePercent: number): number {
    return sumUnreadRebate(inboxStore.messages, seenSeq.value, ratePercent)
  }

  /** 按被邀请人拆分的未读返利额（用于列表里展示每个人本次新增返利）。 */
  function unreadRebatePerInvitee(ratePercent: number): Map<number, number> {
    return unreadRebateByInvitee(inboxStore.messages, seenSeq.value, ratePercent)
  }

  /**
   * 标记"已查看"：推进独立水位并持久化，同时推进 inbox 累积 ack 保持信箱已读语义一致。
   * 进入返利页并离开时调用；调用后菜单红点与页面红箭头一起熄灭。
   */
  function markSeen(): void {
    ensureLoaded(uid.value)
    const seq = latestRechargeSeq(inboxStore.messages)
    if (seq > seenSeq.value) {
      seenSeq.value = seq
      try {
        localStorage.setItem(storageKey(uid.value), String(seq))
      } catch {
        /* localStorage 不可用时静默降级 */
      }
    }
    if (seq > 0) {
      void inboxStore.markReadUpTo(seq)
    }
  }

  return {
    unreadInviteeIDs,
    hasUnread,
    latestSeq,
    unreadRebateTotal,
    unreadRebatePerInvitee,
    markSeen,
  }
}
