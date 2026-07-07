/**
 * Usage request scheduler.
 *
 * All platforms execute immediately without queuing — the backend uses
 * passive sampling so upstream 429 rate-limit errors are no longer a concern.
 */

import type { Account } from '@/types'

/**
 * Schedule a usage fetch. All requests execute immediately.
 */
export function enqueueUsageRequest<T>(
  _account: Account,
  fn: () => Promise<T>
): Promise<T> {
  return fn()
}
