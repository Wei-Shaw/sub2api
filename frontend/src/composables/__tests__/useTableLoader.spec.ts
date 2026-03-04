import { describe, it, expect, vi, beforeEach, afterEach REDACTED from 'vitest'
import { useTableLoader REDACTED from '@/composables/useTableLoader'

// Mock @vueuse/core 的 useDebounceFn
vi.mock('@vueuse/core', () => ({
  useDebounceFn: (fn: Function, ms: number) => {
    let timer: ReturnType<typeof setTimeout> | null = null
    const debounced = (...args: any[]) => {
      if (timer) clearTimeout(timer)
      timer = setTimeout(() => fn(...args), ms)
    REDACTED
    debounced.cancel = () => { if (timer) clearTimeout(timer) REDACTED
    return debounced
  REDACTED,
REDACTED))

// Mock Vue 的 onUnmounted（composable 外使用时会报错）
vi.mock('vue', async () => {
  const actual = await vi.importActual('vue')
  return {
    ...actual,
    onUnmounted: vi.fn(),
  REDACTED
REDACTED)

const createMockFetchFn = (items: any[] = [], total = 0, pages = 1) => {
  return vi.fn().mockResolvedValue({ items, total, pages REDACTED)
REDACTED

describe('useTableLoader', () => {
  beforeEach(() => {
    vi.useFakeTimers()
    vi.clearAllMocks()
  REDACTED)

  afterEach(() => {
    vi.useRealTimers()
  REDACTED)

  // --- 基础加载 ---

  describe('基础加载', () => {
    it('load 执行 fetchFn 并更新 items', async () => {
      const mockItems = [{ id: 1, name: 'item1' REDACTED, { id: 2, name: 'item2' REDACTED]
      const fetchFn = createMockFetchFn(mockItems, 2, 1)

      const { items, loading, load, pagination REDACTED = useTableLoader({
        fetchFn,
      REDACTED)

      expect(items.value).toHaveLength(0)

      await load()

      expect(items.value).toEqual(mockItems)
      expect(pagination.total).toBe(2)
      expect(pagination.pages).toBe(1)
      expect(loading.value).toBe(false)
    REDACTED)

    it('load 期间 loading 为 true', async () => {
      let resolveLoad: (v: any) => void
      const fetchFn = vi.fn(
        () => new Promise((resolve) => { resolveLoad = resolve REDACTED)
      )

      const { loading, load REDACTED = useTableLoader({ fetchFn REDACTED)

      const p = load()
      expect(loading.value).toBe(true)

      resolveLoad!({ items: [], total: 0, pages: 0 REDACTED)
      await p

      expect(loading.value).toBe(false)
    REDACTED)

    it('使用默认 pageSize=20', async () => {
      const fetchFn = createMockFetchFn()
      const { load, pagination REDACTED = useTableLoader({ fetchFn REDACTED)

      await load()

      expect(fetchFn).toHaveBeenCalledWith(
        1,
        20,
        expect.anything(),
        expect.objectContaining({ signal: expect.any(AbortSignal) REDACTED)
      )
      expect(pagination.page_size).toBe(20)
    REDACTED)

    it('可自定义 pageSize', async () => {
      const fetchFn = createMockFetchFn()
      const { load REDACTED = useTableLoader({ fetchFn, pageSize: 50 REDACTED)

      await load()

      expect(fetchFn).toHaveBeenCalledWith(
        1,
        50,
        expect.anything(),
        expect.anything()
      )
    REDACTED)
  REDACTED)

  // --- 分页 ---

  describe('分页', () => {
    it('handlePageChange 更新页码并加载', async () => {
      const fetchFn = createMockFetchFn([], 100, 5)
      const { handlePageChange, pagination, load REDACTED = useTableLoader({ fetchFn REDACTED)

      await load() // 初始加载
      fetchFn.mockClear()

      handlePageChange(3)

      expect(pagination.page).toBe(3)
      // 等待 load 完成
      await vi.runAllTimersAsync()
      expect(fetchFn).toHaveBeenCalledWith(3, 20, expect.anything(), expect.anything())
    REDACTED)

    it('handlePageSizeChange 重置到第1页并加载', async () => {
      const fetchFn = createMockFetchFn([], 100, 5)
      const { handlePageSizeChange, pagination, load REDACTED = useTableLoader({ fetchFn REDACTED)

      await load()
      pagination.page = 3
      fetchFn.mockClear()

      handlePageSizeChange(50)

      expect(pagination.page).toBe(1)
      expect(pagination.page_size).toBe(50)
    REDACTED)

    it('handlePageChange 限制页码范围', async () => {
      const fetchFn = createMockFetchFn([], 100, 5)
      const { handlePageChange, pagination, load REDACTED = useTableLoader({ fetchFn REDACTED)

      await load()

      // 超出范围的页码被限制
      handlePageChange(999)
      expect(pagination.page).toBe(5) // 限制在 pages=5

      handlePageChange(0)
      expect(pagination.page).toBe(1) // 最小为 1
    REDACTED)
  REDACTED)

  // --- 搜索防抖 ---

  describe('搜索防抖', () => {
    it('debouncedReload 在 300ms 内多次调用只执行一次', async () => {
      const fetchFn = createMockFetchFn()
      const { debouncedReload REDACTED = useTableLoader({ fetchFn REDACTED)

      // 快速连续调用
      debouncedReload()
      debouncedReload()
      debouncedReload()

      // 还没到 300ms，不应调用 fetchFn
      expect(fetchFn).not.toHaveBeenCalled()

      // 推进 300ms
      vi.advanceTimersByTime(300)

      // 等待异步完成
      await vi.runAllTimersAsync()

      expect(fetchFn).toHaveBeenCalledTimes(1)
    REDACTED)

    it('reload 重置到第 1 页', async () => {
      const fetchFn = createMockFetchFn([], 100, 5)
      const { reload, pagination, load REDACTED = useTableLoader({ fetchFn REDACTED)

      await load()
      pagination.page = 3

      await reload()

      expect(pagination.page).toBe(1)
    REDACTED)
  REDACTED)

  // --- 请求取消 ---

  describe('请求取消', () => {
    it('新请求取消前一个未完成的请求', async () => {
      let callCount = 0
      const fetchFn = vi.fn((_page, _size, _params, options) => {
        callCount++
        const currentCall = callCount
        return new Promise((resolve, reject) => {
          // 模拟监听 abort
          if (options?.signal) {
            options.signal.addEventListener('abort', () => {
              reject({ name: 'CanceledError', code: 'ERR_CANCELED' REDACTED)
            REDACTED)
          REDACTED
          // 异步解决
          setTimeout(() => {
            resolve({ items: [{ id: currentCall REDACTED], total: 1, pages: 1 REDACTED)
          REDACTED, 1000)
        REDACTED)
      REDACTED)

      const { load REDACTED = useTableLoader({ fetchFn REDACTED)

      // 第一次加载
      const p1 = load()
      // 第二次加载（应取消第一次）
      const p2 = load()

      // 推进时间让第二次完成
      vi.advanceTimersByTime(1000)
      await vi.runAllTimersAsync()

      // 等待两个 Promise settle
      await Promise.allSettled([p1, p2])

      // 第二次请求的结果生效
      expect(fetchFn).toHaveBeenCalledTimes(2)
    REDACTED)
  REDACTED)

  // --- 错误处理 ---

  describe('错误处理', () => {
    it('非取消错误会被抛出', async () => {
      const fetchFn = vi.fn().mockRejectedValue(new Error('Server error'))
      const { load REDACTED = useTableLoader({ fetchFn REDACTED)

      await expect(load()).rejects.toThrow('Server error')
    REDACTED)

    it('取消错误被静默处理', async () => {
      const fetchFn = vi.fn().mockRejectedValue({ name: 'CanceledError', code: 'ERR_CANCELED' REDACTED)
      const { load REDACTED = useTableLoader({ fetchFn REDACTED)

      // 不应抛出
      await load()
    REDACTED)
  REDACTED)
REDACTED)
