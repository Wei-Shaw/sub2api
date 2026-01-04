import { ref, reactive, onUnmounted REDACTED from 'vue'
import { useDebounceFn REDACTED from '@vueuse/core'

interface PaginationState {
  page: number
  page_size: number
  total: number
  pages: number
REDACTED

interface TableLoaderOptions<T, P> {
  fetchFn: (page: number, pageSize: number, params: P, options?: { signal: AbortSignal REDACTED) => Promise<{
    items: T[]
    total: number
    pages: number
  REDACTED>
  initialParams?: P
  pageSize?: number
  debounceMs?: number
REDACTED

export function useTableLoader<T, P extends Record<string, any>>(options: TableLoaderOptions<T, P>) {
  const { fetchFn, initialParams, pageSize = 20, debounceMs = 300 REDACTED = options

  const items = ref<T[]>([])
  const loading = ref(false)
  const params = reactive<P>({ ...(initialParams || {REDACTED) REDACTED as P)
  const pagination = reactive<PaginationState>({
    page: 1,
    page_size: pageSize,
    total: 0,
    pages: 0
  REDACTED)

  let abortController: AbortController | null = null

  const isAbortError = (error: any) => {
    return error?.name === 'AbortError' || error?.code === 'ERR_CANCELED'
  REDACTED

  const load = async () => {
    if (abortController) {
      abortController.abort()
    REDACTED
    abortController = new AbortController()
    loading.value = true

    try {
      const response = await fetchFn(
        pagination.page,
        pagination.page_size,
        params,
        { signal: abortController.signal REDACTED
      )
      
      items.value = response.items
      pagination.total = response.total
      pagination.pages = response.pages
    REDACTED catch (error) {
      if (!isAbortError(error)) {
        throw error
      REDACTED
    REDACTED finally {
      if (abortController?.signal.aborted === false) {
        loading.value = false
      REDACTED
    REDACTED
  REDACTED

  const reload = () => {
    pagination.page = 1
    return load()
  REDACTED

  const debouncedLoad = useDebounceFn(reload, debounceMs)

  const handlePageChange = (page: number) => {
    pagination.page = page
    load()
  REDACTED

  const handlePageSizeChange = (size: number) => {
    pagination.page_size = size
    reload()
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
    debouncedLoad,
    handlePageChange,
    handlePageSizeChange
  REDACTED
REDACTED
