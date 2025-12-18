/**
 * Redeem code API endpoints
 * Handles redeem code redemption for users
 */

import { apiClient REDACTED from './client';
import type { RedeemCodeRequest REDACTED from '@/types';

export interface RedeemHistoryItem {
  id: number;
  code: string;
  type: string;
  value: number;
  status: string;
  used_at: string;
  created_at: string;
  // 订阅类型专用字段
  group_id?: number;
  validity_days?: number;
  group?: {
    id: number;
    name: string;
  REDACTED;
REDACTED

/**
 * Redeem a code
 * @param code - Redeem code string
 * @returns Redemption result with updated balance or concurrency
 */
export async function redeem(code: string): Promise<{
  message: string;
  type: string;
  value: number;
  new_balance?: number;
  new_concurrency?: number;
REDACTED> {
  const payload: RedeemCodeRequest = { code REDACTED;

  const { data REDACTED = await apiClient.post<{
    message: string;
    type: string;
    value: number;
    new_balance?: number;
    new_concurrency?: number;
  REDACTED>('/redeem', payload);

  return data;
REDACTED

/**
 * Get user's redemption history
 * @returns List of redeemed codes
 */
export async function getHistory(): Promise<RedeemHistoryItem[]> {
  const { data REDACTED = await apiClient.get<RedeemHistoryItem[]>('/redeem/history');
  return data;
REDACTED

export const redeemAPI = {
  redeem,
  getHistory,
REDACTED;

export default redeemAPI;
