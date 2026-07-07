import { getCurrentInstance, onUnmounted } from 'vue'

export interface KeyedDebouncedSearchContext {
  key: string
  signal: AbortSignal
}

interface UseKeyedDebouncedSearchOptions<T> {
  delay?: number
  search: (keyword: string, context: KeyedDebouncedSearchContext) => Promise<T>
  onSuccess: (key: string, result: T) => void
  onError?: (key: string, error: unknown) => void
}

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
    }

    const controller = controllers.get(key)
    if (controller) {
      controller.abort()
      controllers.delete(key)
    }

    versions.delete(key)
  }

  const clearAll = () => {
    const allKeys = new Set<string>([
      ...timers.keys(),
      ...controllers.keys(),
      ...versions.keys()
    ])

    allKeys.forEach((key) => clearKey(key))
  }

  const trigger = (key: string, keyword: string) => {
    const nextVersion = (versions.get(key) ?? 0) + 1
    versions.set(key, nextVersion)

    const existingTimer = timers.get(key)
    if (existingTimer) {
      clearTimeout(existingTimer)
      timers.delete(key)
    }

    const inFlight = controllers.get(key)
    if (inFlight) {
      inFlight.abort()
      controllers.delete(key)
    }

    const timer = setTimeout(async () => {
      timers.delete(key)

      const controller = new AbortController()
      controllers.set(key, controller)
      const requestVersion = versions.get(key)

      try {
        const result = await options.search(keyword, { key, signal: controller.signal })
        if (controller.signal.aborted) return
        if (versions.get(key) !== requestVersion) return
        options.onSuccess(key, result)
      } catch (error) {
        if (controller.signal.aborted) return
        if (versions.get(key) !== requestVersion) return
        options.onError?.(key, error)
      } finally {
        if (controllers.get(key) === controller) {
          controllers.delete(key)
        }
      }
    }, delay)

    timers.set(key, timer)
  }

  if (getCurrentInstance()) {
    onUnmounted(() => {
      clearAll()
    })
  }

  return {
    trigger,
    clearKey,
    clearAll
  }
}
