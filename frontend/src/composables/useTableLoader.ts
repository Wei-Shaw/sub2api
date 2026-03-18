import { ref, reactive, onUnmounted, toRaw REDACTED from 'vue'
import { useDebounceFn REDACTED from '@vueuse/core'
import type { BasePaginationResponse, FetchOptions REDACTED from '@/types'
import { getPersistedPageSize, setPersistedPageSize REDACTED from './usePersistedPageSize'

interface PaginationState {
  page: number
  page_size: number
  total: number
  pages: number
REDACTED

interface TableLoaderOptions<T, P> {
  fetchFn: (page: number, pageSize: number, params: P, options?: FetchOptions) => Promise<BasePaginationResponse<T>>
  initialParams?: P
  pageSize?: number
  debounceMs?: number
REDACTED

/**
 * 通用表格数据加载 Composable
 * 统一处理分页、筛选、搜索防抖和请求取消
 */
export function useTableLoader<T, P extends Record<string, any>>(options: TableLoaderOptions<T, P>) {
  const { fetchFn, initialParams, pageSize, debounceMs = 300 REDACTED = options

  const items = ref<T[]>([])
  const loading = ref(false)
  const params = reactive<P>({ ...(initialParams || {REDACTED) REDACTED as P)
  const pagination = reactive<PaginationState>({
    page: 1,
    page_size: pageSize ?? getPersistedPageSize(),
    total: 0,
    pages: 0
  REDACTED)

  let abortController: AbortController | null = null

  const isAbortError = (error: any) => {
    return error?.name === 'AbortError' || error?.code === 'ERR_CANCELED' || error?.name === 'CanceledError'
  REDACTED

  const load = async () => {
    if (abortController) {
      abortController.abort()
    REDACTED
    const currentController = new AbortController()
    abortController = currentController
    loading.value = true

    try {
      const response = await fetchFn(
        pagination.page,
        pagination.page_size,
        toRaw(params) as P,
        { signal: currentController.signal REDACTED
      )

      items.value = response.items || []
      pagination.total = response.total || 0
      pagination.pages = response.pages || 0
    REDACTED catch (error) {
      if (!isAbortError(error)) {
        console.error('Table load error:', error)
        throw error
      REDACTED
    REDACTED finally {
      if (abortController === currentController) {
        loading.value = false
      REDACTED
    REDACTED
  REDACTED

  const reload = () => {
    pagination.page = 1
    return load()
  REDACTED

  const debouncedReload = useDebounceFn(reload, debounceMs)

  const handlePageChange = (page: number) => {
    // 确保页码在有效范围内
    const validPage = Math.max(1, Math.min(page, pagination.pages || 1))
    pagination.page = validPage
    load()
  REDACTED

  const handlePageSizeChange = (size: number) => {
    pagination.page_size = size
    pagination.page = 1
    setPersistedPageSize(size)
    load()
  REDACTED

  onUnmounted(() => {
    abortController?.abort()
  REDACTED)

  return {
    items,
    loading,
    params,
    pagination,
    load,
    reload,
    debouncedReload,
    handlePageChange,
    handlePageSizeChange
  REDACTED
REDACTED
