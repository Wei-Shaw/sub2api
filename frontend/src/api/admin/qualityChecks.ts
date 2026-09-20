import { apiClient } from '../client'
export interface QualityTicket { status: string; account_id?: number; model?: string; length?: number; captured_at?: string; expires_at?: string }
export interface QualityCase { kind: string; status: string; note: string; text: string; model?: string; answer?: number; latency_ms: number; characters: number; ticket?: QualityTicket }
export interface QualityRun { id: number; account_id: number; model: string; status: string; cases: QualityCase[]; started_at: string; finished_at: string | null }
export interface QualityAccount { id: number; name: string; platform: string; status: string; eligible: boolean; next_run_at: string | null; latest: QualityRun | null }
export interface QualityConfig { enabled: boolean; model: string; interval_seconds: number }
export interface QualityOverview { config: QualityConfig; accounts: QualityAccount[]; server_time: string; goose_prompt: string; candy_prompt: string }
const base = '/admin/quality-checks'
export const getQualityOverview = async () => (await apiClient.get<QualityOverview>(base)).data
export const configureQuality = async (config: QualityConfig) => (await apiClient.put(base + '/config', config)).data
export const runQuality = async (accountId = 0) => (await apiClient.post<{ started: number }>(base + '/run', { account_id: accountId })).data
export const getQualityHistory = async (id: number) => (await apiClient.get<QualityRun[]>(`${base}/accounts/${id}/history`)).data
export const getQualityRun = async (id: number) => (await apiClient.get<QualityRun>(`${base}/runs/${id}`)).data
