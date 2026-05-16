/**
 * Module-scoped axios client, injected by install() from HostSdk.
 * All plugin API calls go through this shared instance which carries
 * the host's auth headers and error interceptors.
 */
import type { AxiosInstance } from 'axios'

let _client: AxiosInstance | null = null

export function setClient(client: AxiosInstance): void {
  _client = client
}

export function getClient(): AxiosInstance {
  if (!_client) {
    throw new Error('[gateway-antigravity] API client not initialized. Was install() called?')
  }
  return _client
}
