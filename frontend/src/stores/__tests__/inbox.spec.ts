/**
 * inbox.spec.ts —— useInboxStore（通用信箱前端核心）单元测试。
 *
 * 覆盖：
 *  - longestContiguousEnd / buildInboxWsUrl 纯函数
 *  - renderMessage 去重（seenSeqs）+ 展示列表 prepend
 *  - 累积 ack：300ms 防抖合并、只 ack 连续段末端、非连续留缺口
 *  - markAllRead 立即 ack 到最高 seq
 *  - handleFrame kicked → 置遮罩并停连
 *  - catchup 抬升水位 + 渲染 + has_more 续拉
 *  - localStorage 持久化（ack_seq / seen_seqs）与重载
 *  - reset 清空内存状态
 */
import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { setActivePinia, createPinia } from 'pinia'

const { getInboxCatchup, postInboxAck, getInboxUnreadCount } = vi.hoisted(() => ({
  getInboxCatchup: vi.fn(),
  postInboxAck: vi.fn(),
  getInboxUnreadCount: vi.fn(),
}))

vi.mock('@/api/inbox', () => ({
  getInboxCatchup,
  postInboxAck,
  getInboxUnreadCount,
}))

vi.mock('@/api/url', () => ({
  getAPIBaseURL: () => '/api/v1',
}))

const authStub = { user: { id: 1 } as { id: number } | null, token: 'tok' as string | null }
vi.mock('@/stores/auth', () => ({
  useAuthStore: () => authStub,
}))

// ---- Mock WebSocket ----
class MockWebSocket {
  static CONNECTING = 0
  static OPEN = 1
  static CLOSING = 2
  static CLOSED = 3
  static instances: MockWebSocket[] = []

  url: string
  readyState = MockWebSocket.CONNECTING
  onopen: (() => void) | null = null
  onmessage: ((ev: { data: string }) => void) | null = null
  onclose: (() => void) | null = null
  onerror: (() => void) | null = null

  constructor(url: string) {
    this.url = url
    MockWebSocket.instances.push(this)
  }
  open() {
    this.readyState = MockWebSocket.OPEN
    this.onopen?.()
  }
  emit(data: unknown) {
    this.onmessage?.({ data: typeof data === 'string' ? data : JSON.stringify(data) })
  }
  close() {
    this.readyState = MockWebSocket.CLOSED
    this.onclose?.()
  }
}

import {
  useInboxStore,
  longestContiguousEnd,
  buildInboxWsUrl,
} from '@/stores/inbox'

describe('inbox pure helpers', () => {
  it('longestContiguousEnd 找连续段末端', () => {
    expect(longestContiguousEnd(0, [1, 2, 3])).toBe(3)
    expect(longestContiguousEnd(0, [1, 2, 4, 5])).toBe(2) // 缺 3
    expect(longestContiguousEnd(5, [6, 7, 9])).toBe(7)
    expect(longestContiguousEnd(5, [7, 8])).toBe(5) // 缺 6，不推进
    expect(longestContiguousEnd(0, [])).toBe(0)
  })

  it('buildInboxWsUrl 相对 base 用 origin 且换 ws 协议', () => {
    const u = buildInboxWsUrl('/api/v1', 'abc', 'https://ex.com')
    expect(u).toBe('wss://ex.com/api/v1/inbox/ws?token=abc')
    const u2 = buildInboxWsUrl('http://h:8080/api/v1', 'x')
    expect(u2).toBe('ws://h:8080/api/v1/inbox/ws?token=x')
  })
})

describe('useInboxStore', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
    vi.useFakeTimers()
    vi.clearAllMocks()
    localStorage.clear()
    MockWebSocket.instances = []
    vi.stubGlobal('WebSocket', MockWebSocket)
    authStub.user = { id: 1 }
    authStub.token = 'tok'
    postInboxAck.mockResolvedValue({ acked_seq: 0 })
    getInboxCatchup.mockResolvedValue({ messages: [], acked_seq: 0, has_more: false })
    getInboxUnreadCount.mockResolvedValue({ count: 0, truncated: false })
  })
  afterEach(() => {
    vi.useRealTimers()
    vi.unstubAllGlobals()
  })

  it('renderMessage 去重 + prepend + 计未读', () => {
    const store = useInboxStore()
    const mk = (seq: number) => ({
      seq,
      scope: 'direct' as const,
      namespace: 'support_ticket',
      payload: {},
      created_at: '2026-07-18T00:00:00Z',
    })
    store.renderMessage(mk(1))
    store.renderMessage(mk(2))
    store.renderMessage(mk(2)) // 重复
    expect(store.messages.length).toBe(2)
    expect(store.messages[0].seq).toBe(2) // 最新在前
    expect(store.unreadCount).toBe(2)
    expect(store.seenSeqs).toEqual([1, 2])
  })

  it('累积 ack：300ms 内多条只 ack 一次（连续段末端）', async () => {
    postInboxAck.mockResolvedValue({ acked_seq: 3 })
    const store = useInboxStore()
    const mk = (seq: number) => ({ seq, scope: 'direct' as const, namespace: 'n', payload: {}, created_at: '' })
    store.renderMessage(mk(1))
    store.renderMessage(mk(2))
    store.renderMessage(mk(3))
    expect(postInboxAck).not.toHaveBeenCalled() // 防抖内未发

    await vi.advanceTimersByTimeAsync(300)
    expect(postInboxAck).toHaveBeenCalledTimes(1)
    expect(postInboxAck).toHaveBeenCalledWith(3)
    expect(store.localAckSeq).toBe(3)
    expect(store.seenSeqs).toEqual([]) // 全部 ack 后裁剪
    expect(store.unreadCount).toBe(0)
  })

  it('非连续 seq 只 ack 到缺口前', async () => {
    postInboxAck.mockImplementation((n: number) => Promise.resolve({ acked_seq: n }))
    const store = useInboxStore()
    const mk = (seq: number) => ({ seq, scope: 'direct' as const, namespace: 'n', payload: {}, created_at: '' })
    store.renderMessage(mk(1))
    store.renderMessage(mk(2))
    store.renderMessage(mk(4)) // 缺 3
    await vi.advanceTimersByTimeAsync(300)
    expect(postInboxAck).toHaveBeenCalledWith(2)
    expect(store.localAckSeq).toBe(2)
    expect(store.seenSeqs).toEqual([4]) // 4 仍未 ack
  })

  it('markAllRead 立即 ack 到最高 seq', async () => {
    postInboxAck.mockImplementation((n: number) => Promise.resolve({ acked_seq: n }))
    const store = useInboxStore()
    const mk = (seq: number) => ({ seq, scope: 'direct' as const, namespace: 'n', payload: {}, created_at: '' })
    store.renderMessage(mk(1))
    store.renderMessage(mk(3)) // 非连续
    await store.markAllRead()
    expect(postInboxAck).toHaveBeenCalledWith(3)
    expect(store.localAckSeq).toBe(3)
    expect(store.unreadCount).toBe(0)
  })

  it('flushNow 跳过防抖立即 ack 连续段末端', async () => {
    postInboxAck.mockImplementation((n: number) => Promise.resolve({ acked_seq: n }))
    const store = useInboxStore()
    const mk = (seq: number) => ({ seq, scope: 'direct' as const, namespace: 'n', payload: {}, created_at: '' })
    store.renderMessage(mk(1))
    store.renderMessage(mk(2))
    await store.flushNow()
    expect(postInboxAck).toHaveBeenCalledWith(2)
    expect(store.localAckSeq).toBe(2)
    // flushNow 后防抖 timer 已清，再推进时间不应重复 ack
    await vi.advanceTimersByTimeAsync(300)
    expect(postInboxAck).toHaveBeenCalledTimes(1)
  })

  it('handleFrame kicked → 置遮罩', () => {
    const store = useInboxStore()
    store.handleFrame(JSON.stringify({ type: 'kicked', reason: 'opened_elsewhere', client_type: 'web' }))
    expect(store.kicked).toBe(true)
    expect(store.kickedReason).toBe('opened_elsewhere')
  })

  it('catchup 抬升水位并渲染 + has_more 续拉', async () => {
    getInboxCatchup
      .mockResolvedValueOnce({
        messages: [
          { seq: 1, scope: 'direct', namespace: 'n', payload: {}, created_at: '' },
          { seq: 2, scope: 'broadcast', namespace: 'n', payload: {}, created_at: '' },
        ],
        acked_seq: 0,
        has_more: true,
      })
      .mockResolvedValueOnce({
        messages: [{ seq: 3, scope: 'direct', namespace: 'n', payload: {}, created_at: '' }],
        acked_seq: 0,
        has_more: false,
      })
    const store = useInboxStore()
    await store.catchup(0)
    expect(getInboxCatchup).toHaveBeenCalledTimes(2)
    expect(getInboxCatchup).toHaveBeenNthCalledWith(2, 2)
    expect(store.messages.length).toBe(3)
    expect(store.seenSeqs).toEqual([1, 2, 3])
  })

  it('catchup 服务端水位更高时丢弃已 ack 消息', async () => {
    getInboxCatchup.mockResolvedValue({
      messages: [{ seq: 5, scope: 'direct', namespace: 'n', payload: {}, created_at: '' }],
      acked_seq: 10,
      has_more: false,
    })
    const store = useInboxStore()
    await store.catchup(0)
    expect(store.localAckSeq).toBe(10)
    expect(store.messages.length).toBe(0) // seq 5 <= 10 被丢弃
  })

  it('持久化：ack 后写 localStorage，重建 store 能重载', async () => {
    postInboxAck.mockResolvedValue({ acked_seq: 2 })
    const store = useInboxStore()
    const mk = (seq: number) => ({ seq, scope: 'direct' as const, namespace: 'n', payload: {}, created_at: '' })
    store.renderMessage(mk(1))
    store.renderMessage(mk(2))
    store.renderMessage(mk(5))
    await vi.advanceTimersByTimeAsync(300)
    expect(localStorage.getItem('inbox:1:ack_seq')).toBe('2')
    expect(JSON.parse(localStorage.getItem('inbox:1:seen_seqs') || '[]')).toEqual([5])
  })

  it('reset 清空内存状态', () => {
    const store = useInboxStore()
    store.renderMessage({ seq: 1, scope: 'direct', namespace: 'n', payload: {}, created_at: '' })
    store.reset()
    expect(store.messages).toEqual([])
    expect(store.seenSeqs).toEqual([])
    expect(store.localAckSeq).toBe(0)
    expect(store.kicked).toBe(false)
  })

  it('connect 建立 WS，收到 push 渲染', async () => {
    const store = useInboxStore()
    store.connect()
    expect(MockWebSocket.instances.length).toBe(1)
    const ws = MockWebSocket.instances[0]
    // 相对 base + jsdom origin(http://localhost) → ws://（非 wss）
    expect(ws.url).toContain('ws://')
    expect(ws.url).toContain('/api/v1/inbox/ws')
    expect(ws.url).toContain('token=tok')
    ws.open()
    expect(store.connected).toBe(true)
    ws.emit({ type: 'notification', seq: 9, scope: 'direct', namespace: 'n', payload: {}, created_at: '' })
    expect(store.messages[0].seq).toBe(9)
  })
})
