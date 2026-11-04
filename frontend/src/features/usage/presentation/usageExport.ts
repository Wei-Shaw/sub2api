import { query } from '../data/datasources/usageDatasource'
import type { UsageQueryParams } from '@/types'

export const USAGE_EXPORT_PAGE_SIZE = 1000

function waitForRetry(seconds: number, signal: AbortSignal): Promise<void> {
  return new Promise((resolve, reject) => {
    const abort = () => {
      clearTimeout(timer)
      signal.removeEventListener('abort', abort)
      reject(new DOMException('Export canceled', 'AbortError'))
    }
    const timer = setTimeout(() => {
      signal.removeEventListener('abort', abort)
      resolve()
    }, seconds * 1000)
    signal.addEventListener('abort', abort, { once: true })
    if (signal.aborted) abort()
  })
}

export async function queryUsageExportPage(
  params: UsageQueryParams,
  signal: AbortSignal,
  onRateLimit: (seconds: number) => void,
) {
  for (let attempt = 0; ; attempt++) {
    signal.throwIfAborted()
    try {
      return await query(params, { signal })
    } catch (error: unknown) {
      const limited = error as { status?: number; retry_after_seconds?: number }
      if (signal.aborted || limited?.status !== 429 || attempt >= 3) throw error
      const retryAfter = limited.retry_after_seconds
      const seconds = typeof retryAfter === 'number' && Number.isFinite(retryAfter)
        ? Math.min(Math.max(retryAfter, 1), 60)
        : 60
      onRateLimit(seconds)
      await waitForRetry(seconds, signal)
    }
  }
}
