/**
 * inbox.ts —— 通用信箱（general-inbox）前端核心 store（PR-5）。
 *
 * 一致性模型（与后端 design.md 对齐）：
 *   - 存储严格一次：后端 DB 唯一约束；
 *   - 推送至少一次：WS 实时 + REST catchup 兜底；
 *   - 消费恰好一次：客户端 `seenSeqs` 幂等去重 + 累积 ack 水位单调推进。
 *
 * 关键机制：
 *   1. 冷启动 bootstrap：加载 localStorage 的 local_ack_seq / seen_seqs → 建 WS
 *      → catchup(since=local_ack_seq) → 回放 bootstrapping 期间 buffer 的 WS 推送。
 *   2. 累积 ack + 连续段推进：从 local_ack_seq+1 起在 seenSeqs 找最长连续段末端 n，
 *      300ms defer 合并后只打一次 ack(n)（密集消息只 ack 一次）。
 *   3. 持久化：`inbox:{uid}:ack_seq` + `inbox:{uid}:seen_seqs`，
 *      render 后立即持久化 seenSeqs，server ack 200 后持久化 ack_seq 并裁剪 seenSeqs。
 *   4. 单例连接踢出：收到 {type:"kicked"} → 展示遮罩、停止自动重连；
 *      网络异常（非 kicked）走指数退避重连。
 *   5. logout：auth 侧调用 reset() → 断开连接、清定时器、清内存状态（localStorage 按
 *      uid 保留，便于同一用户再次登录续上水位）。
 *
 * 该 store 只做"信箱协议"，不关心具体业务渲染；payload 由 namespace 交给 UI 层。
 */
import { computed, ref } from 'vue'
import { defineStore } from 'pinia'

import {
  getInboxCatchup,
  getInboxUnreadCount,
  postInboxAck,
  type InboxMessage,
} from '@/api/inbox'
import { getAPIBaseURL } from '@/api/url'
import { useAuthStore } from '@/stores/auth'

/** ack 防抖窗口：多条 push 300ms 内到达只发一次 ack(max)。 */
const ACK_DEBOUNCE_MS = 300

/** 展示列表上限，防离线堆积把内存/DOM 打爆；只保留最新 N 条。 */
const MAX_MESSAGES = 200

/** 重连退避：base 1s，指数增长，封顶 30s；成功 open 后重置。 */
const RECONNECT_BASE_MS = 1_000
const RECONNECT_MAX_MS = 30_000

/** localStorage token key（与 auth store 一致）。 */
const AUTH_TOKEN_KEY = 'auth_token'

// ============================================================
// 纯函数 helpers（可独立测试）
// ============================================================

/** 从 ackSeq+1 起，在升序 seen 列表中寻找最长连续段的末端 seq。 */
export function longestContiguousEnd(ackSeq: number, sortedSeen: number[]): number {
  let n = ackSeq
  let expect = ackSeq + 1
  for (const s of sortedSeen) {
    if (s < expect) continue
    if (s === expect) {
      n = s
      expect++
    } else {
      break
    }
  }
  return n
}

/** 构造 inbox WebSocket URL：把 http(s) base 换成 ws(s)，token 走 query。 */
export function buildInboxWsUrl(base: string, token: string, origin?: string): string {
  let httpBase = base
  if (!/^https?:\/\//i.test(base)) {
    const o = origin ?? (typeof window !== 'undefined' ? window.location.origin : '')
    httpBase = `${o}${base.startsWith('/') ? '' : '/'}${base}`
  }
  const u = new URL(`${httpBase.replace(/\/+$/, '')}/inbox/ws`)
  u.protocol = u.protocol === 'https:' ? 'wss:' : 'ws:'
  u.searchParams.set('token', token)
  return u.toString()
}

// ============================================================
// Store
// ============================================================

export const useInboxStore = defineStore('inbox', () => {
  // ==================== State ====================

  /** 展示消息列表（最新在前），上限 MAX_MESSAGES。 */
  const messages = ref<InboxMessage[]>([])

  /** 已 ack 水位；持久化为 inbox:{uid}:ack_seq。 */
  const localAckSeq = ref(0)

  /**
   * 已 render 但尚未 ack 的 seq（升序、去重、均 > localAckSeq）。
   * 持久化为 inbox:{uid}:seen_seqs，用于刷新后去重 + ack 连续段推进。
   */
  const seenSeqs = ref<number[]>([])

  /** WS 是否已连接。 */
  const connected = ref(false)

  /** 冷启动补齐进行中；期间到达的 WS 推送先 buffer，catchup 完成后回放。 */
  const bootstrapping = ref(false)

  /** 被踢出（其他端打开）；置位后展示遮罩且不自动重连。 */
  const kicked = ref(false)
  const kickedReason = ref('')
  const kickedClientType = ref('')

  // ---- 私有闭包状态（不导出） ----
  let socket: WebSocket | null = null
  let manualClose = false
  let reconnectTimer: ReturnType<typeof setTimeout> | null = null
  let reconnectAttempts = 0
  let ackTimer: ReturnType<typeof setTimeout> | null = null
  let pendingAck = 0
  let pushBuffer: InboxMessage[] = []

  // ==================== Getters ====================

  /** 未读数 = 已 render 未 ack 的条数。 */
  const unreadCount = computed(() => seenSeqs.value.length)
  const hasUnread = computed(() => unreadCount.value > 0)

  // ==================== 持久化 ====================

  function uid(): string {
    const auth = useAuthStore()
    const id = auth.user?.id
    return id !== undefined && id !== null ? String(id) : 'anon'
  }

  // key 实时解析 uid，避免"store 创建时用户尚未就绪"导致落到 anon 桶。
  function ackKey(): string {
    return `inbox:${uid()}:ack_seq`
  }
  function seenKey(): string {
    return `inbox:${uid()}:seen_seqs`
  }

  function loadPersisted(): void {
    try {
      const rawAck = localStorage.getItem(ackKey())
      localAckSeq.value = rawAck ? parseInt(rawAck, 10) || 0 : 0
    } catch {
      localAckSeq.value = 0
    }
    try {
      const rawSeen = localStorage.getItem(seenKey())
      const arr = rawSeen ? (JSON.parse(rawSeen) as unknown) : []
      seenSeqs.value = Array.isArray(arr)
        ? (arr.filter((x) => typeof x === 'number' && x > localAckSeq.value) as number[]).sort(
            (a, b) => a - b
          )
        : []
    } catch {
      seenSeqs.value = []
    }
  }

  function persistAck(): void {
    try {
      localStorage.setItem(ackKey(), String(localAckSeq.value))
    } catch {
      /* localStorage 不可用时静默降级 */
    }
  }
  function persistSeen(): void {
    try {
      localStorage.setItem(seenKey(), JSON.stringify(seenSeqs.value))
    } catch {
      /* ignore */
    }
  }

  // ==================== 消息渲染 + 去重 ====================

  /**
   * renderMessage 处理一条消息（来自 catchup 或 WS）。
   *  - seq <= localAckSeq：早已 ack，丢弃；
   *  - 已在 seenSeqs：重复推送，跳过 render 但仍参与 ack 推进；
   *  - 新 seq：插入 seenSeqs（保持升序）、prepend 到展示列表、持久化。
   */
  function renderMessage(msg: InboxMessage): void {
    if (msg.seq <= localAckSeq.value) return
    if (seenSeqs.value.includes(msg.seq)) {
      tryScheduleAck()
      return
    }
    // 有序插入 seenSeqs
    const seen = seenSeqs.value
    let lo = 0
    let hi = seen.length
    while (lo < hi) {
      const mid = (lo + hi) >> 1
      if (seen[mid] < msg.seq) lo = mid + 1
      else hi = mid
    }
    seen.splice(lo, 0, msg.seq)

    // prepend 展示列表并裁剪
    messages.value.unshift(msg)
    if (messages.value.length > MAX_MESSAGES) {
      messages.value.length = MAX_MESSAGES
    }

    persistSeen()
    tryScheduleAck()
  }

  // ==================== 累积 ack ====================

  /** 计算连续段末端并排入 pendingAck，300ms defer 后 flush。 */
  function tryScheduleAck(): void {
    const n = longestContiguousEnd(localAckSeq.value, seenSeqs.value)
    if (n <= localAckSeq.value) return
    pendingAck = n
    if (ackTimer === null) {
      ackTimer = setTimeout(() => {
        void flushAck()
      }, ACK_DEBOUNCE_MS)
    }
  }

  /** 真正打 ack RPC；成功后推进水位、裁剪 seenSeqs 并持久化。 */
  async function flushAck(): Promise<void> {
    ackTimer = null
    const n = pendingAck
    if (n <= localAckSeq.value) return
    try {
      const res = await postInboxAck(n)
      const acked = Math.max(n, res.acked_seq)
      localAckSeq.value = Math.max(localAckSeq.value, acked)
      seenSeqs.value = seenSeqs.value.filter((s) => s > localAckSeq.value)
      persistAck()
      persistSeen()
    } catch (err) {
      console.error('inbox: ack failed, will retry on next message', err)
      // 失败不推进水位；下一条消息或 markAllRead 会重新 schedule。
      tryScheduleAck()
    }
  }

  /** 用户主动"全部已读"：立刻 ack 到当前最高 seen seq。 */
  async function markAllRead(): Promise<void> {
    if (seenSeqs.value.length === 0) return
    const top = seenSeqs.value[seenSeqs.value.length - 1]
    if (top <= localAckSeq.value) return
    if (ackTimer !== null) {
      clearTimeout(ackTimer)
      ackTimer = null
    }
    pendingAck = top
    await flushAck()
  }

  /**
   * markReadUpTo 立即把 ack 水位推进到 seq（累积语义：seq 之前的都算已读）。
   * 用于"点击某条通知"场景——点了第 N 条即代表读到 N。seq<=水位则无操作。
   */
  async function markReadUpTo(seq: number): Promise<void> {
    if (seq <= localAckSeq.value) return
    if (ackTimer !== null) {
      clearTimeout(ackTimer)
      ackTimer = null
    }
    pendingAck = seq
    await flushAck()
  }

  /**
   * flushNow 立即（跳过防抖）把当前连续段末端 ack 出去。
   * 用于页面进入后台（visibilitychange→hidden）等"可能很快被冻结/回收"的时机，
   * 尽量把已读水位落库，减少下次冷启动的重复展示窗口。
   */
  async function flushNow(): Promise<void> {
    const n = longestContiguousEnd(localAckSeq.value, seenSeqs.value)
    if (n <= localAckSeq.value) return
    if (ackTimer !== null) {
      clearTimeout(ackTimer)
      ackTimer = null
    }
    pendingAck = n
    await flushAck()
  }

  /**
   * beaconAck 在 beforeunload/pagehide 时用 fetch(keepalive) 尽力上报 ack。
   * 普通异步请求会被卸载中断，keepalive 允许请求在页面销毁后继续；
   * 需要带 Authorization 头（sendBeacon 无法设置头，故用 fetch 而非 beacon）。
   */
  function beaconAck(): void {
    const n = longestContiguousEnd(localAckSeq.value, seenSeqs.value)
    if (n <= localAckSeq.value) return
    const token = currentToken()
    if (!token) return
    let base = getAPIBaseURL()
    if (!/^https?:\/\//i.test(base) && typeof window !== 'undefined') {
      base = `${window.location.origin}${base.startsWith('/') ? '' : '/'}${base}`
    }
    const url = `${base.replace(/\/+$/, '')}/inbox/ack`
    try {
      void fetch(url, {
        method: 'POST',
        keepalive: true,
        headers: {
          'Content-Type': 'application/json',
          Authorization: `Bearer ${token}`,
        },
        body: JSON.stringify({ seq: n }),
      })
    } catch {
      /* 卸载路径尽力而为，失败忽略 */
    }
  }

  // ==================== catchup ====================

  /** catchup 拉取 since 之后的消息并渲染；has_more 时再补一轮（有限次）。 */
  async function catchup(since: number): Promise<void> {
    let cursor = since
    for (let round = 0; round < 5; round++) {
      const res = await getInboxCatchup(cursor)
      // 服务端权威水位可能高于本地（老设备落后），单调抬升。
      if (res.acked_seq > localAckSeq.value) {
        localAckSeq.value = res.acked_seq
        seenSeqs.value = seenSeqs.value.filter((s) => s > localAckSeq.value)
        persistAck()
        persistSeen()
      }
      for (const m of res.messages) {
        renderMessage(m)
      }
      if (!res.has_more || res.messages.length === 0) break
      cursor = res.messages[res.messages.length - 1].seq
    }
  }

  // ==================== WebSocket ====================

  function currentToken(): string {
    const auth = useAuthStore()
    if (auth.token) return auth.token
    try {
      return localStorage.getItem(AUTH_TOKEN_KEY) ?? ''
    } catch {
      return ''
    }
  }

  function handleFrame(raw: string): void {
    let frame: {
      type?: string
      seq?: number
      scope?: string
      namespace?: string
      payload?: Record<string, unknown>
      created_at?: string
      truncated?: boolean
      unacked?: number[]
      reason?: string
      client_type?: string
    }
    try {
      frame = JSON.parse(raw)
    } catch {
      return
    }

    if (frame.type === 'kicked') {
      kicked.value = true
      kickedReason.value = frame.reason ?? 'opened_elsewhere'
      kickedClientType.value = frame.client_type ?? 'web'
      disconnect()
      return
    }

    // 服务端提示"你还有未 ack 的消息"或未读被截断 → 主动 catchup 兜底。
    if (frame.truncated || (frame.unacked && frame.unacked.length > 0)) {
      if (!bootstrapping.value) {
        void catchup(localAckSeq.value)
      }
      return
    }

    // 普通消息推送
    if (typeof frame.seq === 'number' && frame.seq > 0) {
      const msg: InboxMessage = {
        seq: frame.seq,
        scope: (frame.scope as InboxMessage['scope']) ?? 'direct',
        namespace: frame.namespace ?? '',
        payload: frame.payload ?? {},
        created_at: frame.created_at ?? new Date().toISOString(),
      }
      if (bootstrapping.value) {
        pushBuffer.push(msg)
      } else {
        renderMessage(msg)
      }
    }
  }

  function scheduleReconnect(): void {
    if (manualClose || kicked.value) return
    if (reconnectTimer !== null) return
    const delay = Math.min(RECONNECT_BASE_MS * 2 ** reconnectAttempts, RECONNECT_MAX_MS)
    reconnectAttempts++
    reconnectTimer = setTimeout(() => {
      reconnectTimer = null
      connect()
    }, delay)
  }

  function connect(): void {
    if (kicked.value) return
    const token = currentToken()
    if (!token) return
    if (socket && (socket.readyState === WebSocket.OPEN || socket.readyState === WebSocket.CONNECTING)) {
      return
    }
    manualClose = false
    let url: string
    try {
      url = buildInboxWsUrl(getAPIBaseURL(), token)
    } catch (err) {
      console.error('inbox: build ws url failed', err)
      return
    }
    const ws = new WebSocket(url)
    socket = ws
    ws.onopen = () => {
      connected.value = true
      reconnectAttempts = 0
    }
    ws.onmessage = (ev: MessageEvent) => {
      if (typeof ev.data === 'string') handleFrame(ev.data)
    }
    ws.onclose = () => {
      connected.value = false
      if (socket === ws) socket = null
      scheduleReconnect()
    }
    ws.onerror = () => {
      // onclose 会随后触发重连；这里只需保证不抛。
    }
  }

  function disconnect(): void {
    manualClose = true
    if (reconnectTimer !== null) {
      clearTimeout(reconnectTimer)
      reconnectTimer = null
    }
    if (socket) {
      try {
        socket.close()
      } catch {
        /* ignore */
      }
      socket = null
    }
    connected.value = false
  }

  // ==================== 生命周期 ====================

  let bootstrapped = false
  let lifecycleInstalled = false

  /** 页面进入后台时尽快落 ack 水位。 */
  function onVisibilityChange(): void {
    if (typeof document !== 'undefined' && document.visibilityState === 'hidden') {
      void flushNow()
    }
  }

  /** 安装 visibilitychange / pagehide 监听（幂等）。 */
  function installLifecycleHandlers(): void {
    if (lifecycleInstalled || typeof window === 'undefined') return
    lifecycleInstalled = true
    document.addEventListener('visibilitychange', onVisibilityChange)
    window.addEventListener('pagehide', beaconAck)
    window.addEventListener('beforeunload', beaconAck)
  }

  /** 移除生命周期监听。 */
  function removeLifecycleHandlers(): void {
    if (!lifecycleInstalled || typeof window === 'undefined') return
    lifecycleInstalled = false
    document.removeEventListener('visibilitychange', onVisibilityChange)
    window.removeEventListener('pagehide', beaconAck)
    window.removeEventListener('beforeunload', beaconAck)
  }

  /**
   * bootstrap 冷启动：加载持久化水位 → 建 WS → catchup → 回放 buffer。
   * 幂等：重复调用（除非先 reset）直接返回。
   */
  async function bootstrap(): Promise<void> {
    if (bootstrapped) return
    bootstrapped = true
    kicked.value = false
    loadPersisted()
    installLifecycleHandlers()

    bootstrapping.value = true
    connect()
    try {
      await catchup(localAckSeq.value)
    } catch (err) {
      console.error('inbox: bootstrap catchup failed', err)
    } finally {
      bootstrapping.value = false
      // 回放 bootstrapping 期间 buffer 的推送（renderMessage 去重保证幂等）。
      const buffered = pushBuffer
      pushBuffer = []
      for (const m of buffered) renderMessage(m)
    }
  }

  /** 用户点"在此继续"：清除 kicked 遮罩并主动重连（反过来踢其他端）。 */
  function resume(): void {
    if (!kicked.value) return
    kicked.value = false
    kickedReason.value = ''
    kickedClientType.value = ''
    reconnectAttempts = 0
    connect()
  }

  /** 拉服务端权威未读数（badge 兜底展示，可选）。 */
  async function fetchUnreadCount(): Promise<number> {
    try {
      const res = await getInboxUnreadCount()
      return res.count
    } catch (err) {
      console.error('inbox: fetchUnreadCount failed', err)
      return unreadCount.value
    }
  }

  /** logout 清理：断连、清定时器、清内存状态（localStorage 按 uid 保留）。 */
  function reset(): void {
    disconnect()
    removeLifecycleHandlers()
    if (ackTimer !== null) {
      clearTimeout(ackTimer)
      ackTimer = null
    }
    pendingAck = 0
    pushBuffer = []
    reconnectAttempts = 0
    manualClose = false
    bootstrapped = false
    bootstrapping.value = false
    kicked.value = false
    kickedReason.value = ''
    kickedClientType.value = ''
    messages.value = []
    seenSeqs.value = []
    localAckSeq.value = 0
  }

  return {
    // State
    messages,
    localAckSeq,
    seenSeqs,
    connected,
    bootstrapping,
    kicked,
    kickedReason,
    kickedClientType,
    // Getters
    unreadCount,
    hasUnread,
    // Actions
    bootstrap,
    connect,
    disconnect,
    resume,
    catchup,
    renderMessage,
    markAllRead,
    markReadUpTo,
    flushNow,
    fetchUnreadCount,
    reset,
    // 暴露给测试的内部帧处理
    handleFrame,
  }
})
