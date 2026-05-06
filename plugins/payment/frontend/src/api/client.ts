import type { AxiosInstance } from 'axios'

/**
 * Plugin API client wrapper.
 *
 * The host frontend provides an axios instance with auth interceptors. The
 * plugin receives that instance via setClient() at install time and uses it
 * for all HTTP calls so authentication and error handling stay consistent
 * with the host app.
 *
 * Plugin endpoints live under /api/v1/payment/* and /api/v1/admin/payment/*
 * (the original pre-migration host paths). The host's apiClient is
 * configured with `/api/v1` baseURL, so call sites use paths like
 * `/payment/orders` or `/admin/payment/dashboard`.
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
