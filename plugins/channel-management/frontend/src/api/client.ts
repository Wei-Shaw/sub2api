import type { AxiosInstance } from 'axios'

/**
 * Plugin API client wrapper.
 *
 * The core frontend provides an axios instance with auth interceptors. Plugins
 * receive that instance via setClient() and use it for all HTTP calls so that
 * authentication and error handling stay consistent with the host app.
 *
 * All plugin endpoints live under /api/v1/plugin/channel-management/* — the
 * caller is expected to set baseURL or full paths so requests resolve there.
 */

let client: AxiosInstance | null = null

export function setClient(instance: AxiosInstance): void {
  client = instance
}

export function getClient(): AxiosInstance {
  if (!client) {
    throw new Error('[plugin-channel-management] API client not initialized. Call setClient() during plugin install.')
  }
  return client
}
