import { getCurrentInstance, onUnmounted REDACTED from 'vue'

export interface KeyedDebouncedSearchContext {
  key: string
  signal: AbortSignal
REDACTED

interface UseKeyedDebouncedSearchOptions<T> {
  delay?: number
  search: (keyword: string, context: KeyedDebouncedSearchContext) => Promise<T>
  onSuccess: (key: string, result: T) => void
  onError?: (key: string, error: unknown) => void
REDACTED

/**
 * 多实例隔离的防抖搜索：每个 key 有独立的防抖、请求取消与过期响应保护。
 */
export function useKeyedDebouncedSearch<T>(options: UseKeyedDebouncedSearchOptions<T>) {
  const delay = options.delay ?? 300
  const timers = new Map<string, ReturnType<typeof setTimeout>>()
  const controllers = new Map<string, AbortController>()
  const versions = new Map<string, number>()

  const clearKey = (key: string) => {
    const timer = timers.get(key)
    if (timer) {
      clearTimeout(timer)
      timers.delete(key)
    REDACTED

    const controller = controllers.get(key)
    if (controller) {
      controller.abort()
      controllers.delete(key)
    REDACTED

    versions.delete(key)
  REDACTED

  const clearAll = () => {
    const allKeys = new Set<string>([
      ...timers.keys(),
      ...controllers.keys(),
      ...versions.keys()
    ])

    allKeys.forEach((key) => clearKey(key))
  REDACTED

  const trigger = (key: string, keyword: string) => {
    const nextVersion = (versions.get(key) ?? 0) + 1
    versions.set(key, nextVersion)

    const existingTimer = timers.get(key)
    if (existingTimer) {
      clearTimeout(existingTimer)
      timers.delete(key)
    REDACTED

    const inFlight = controllers.get(key)
    if (inFlight) {
      inFlight.abort()
      controllers.delete(key)
    REDACTED

    const timer = setTimeout(async () => {
      timers.delete(key)

      const controller = new AbortController()
      controllers.set(key, controller)
      const requestVersion = versions.get(key)

      try {
        const result = await options.search(keyword, { key, signal: controller.signal REDACTED)
        if (controller.signal.aborted) return
        if (versions.get(key) !== requestVersion) return
        options.onSuccess(key, result)
      REDACTED catch (error) {
        if (controller.signal.aborted) return
        if (versions.get(key) !== requestVersion) return
        options.onError?.(key, error)
      REDACTED finally {
        if (controllers.get(key) === controller) {
          controllers.delete(key)
        REDACTED
      REDACTED
    REDACTED, delay)

    timers.set(key, timer)
  REDACTED

  if (getCurrentInstance()) {
    onUnmounted(() => {
      clearAll()
    REDACTED)
  REDACTED

  return {
    trigger,
    clearKey,
    clearAll
  REDACTED
REDACTED
