import type { AxiosInstance } from 'axios'

/**
 * Plugin API client wrapper.
 *
 * The host frontend provides an axios instance with auth interceptors. The
 * plugin receives that instance via setClient() at install time and uses it
 * for all HTTP calls so authentication and error handling stay consistent
 * with the host app.
 *
 * Plugin endpoints live under /api/v1/plugin/payment/* — the apiClient is
 * already configured with `/api/v1` baseURL by the host, so call sites use
 * paths like `/plugin/payment/orders`.
 */

let client: AxiosInstance | null = null

export function setClient(instance: AxiosInstance): void {
  client = instance
}

export function getClient(): AxiosInstance {
  if (!client) {
    throw new Error('[plugin-payment] API client not initialized. Call setClient() during plugin install.')
  }
  return client
}
