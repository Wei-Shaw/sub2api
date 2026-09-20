import { apiClient } from '../client'

export interface TrafficSample {
  timestamp: number
  upload_bps: number
  download_bps: number
}

export interface NetworkTraffic extends TrafficSample {
  available: boolean
  ready: boolean
  interfaces: string[]
  total_sent: number
  total_received: number
  samples: TrafficSample[]
}

export async function getNetworkTraffic(signal?: AbortSignal): Promise<NetworkTraffic> {
  const { data } = await apiClient.get<NetworkTraffic>('/admin/system/network-traffic', {
    timeout: 6000,
    signal
  })
  return data
}
