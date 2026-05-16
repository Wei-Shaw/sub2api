import type { AxiosInstance } from 'axios'

let client: AxiosInstance | null = null

export function setClient(instance: AxiosInstance): void {
  client = instance
}

export function getClient(): AxiosInstance {
  if (!client) {
    throw new Error(
      '[plugin-gateway-openai] API client not initialized. Call setClient() during plugin install.',
    )
  }
  return client
}
