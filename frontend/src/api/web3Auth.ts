import { apiClient } from './client'
import type { AuthResponse, TotpLoginResponse } from '@/types'

export type Web3AuthIntent = 'login' | 'register'

export interface Web3ChallengeResponse {
  challenge_token: string
  message: string
  expires_at: string
}

export interface Web3ChallengeRequest {
  address: string
  chain_id: string
  turnstile_token?: string
}

export interface Web3VerifyRequest {
  challenge_token: string
  signature: string
}

export interface Web3RegistrationVerifyRequest extends Web3VerifyRequest {
  username: string
  invitation_code?: string
  promo_code?: string
  aff_code?: string
}

export type Web3LoginResponse = AuthResponse | TotpLoginResponse

export async function createWeb3Challenge(
  intent: Web3AuthIntent,
  request: Web3ChallengeRequest,
): Promise<Web3ChallengeResponse> {
  const { data } = await apiClient.post<Web3ChallengeResponse>(
    `/auth/web3/${intent}/challenge`,
    request,
  )
  return data
}

export async function verifyWeb3Login(request: Web3VerifyRequest): Promise<Web3LoginResponse> {
  const { data } = await apiClient.post<Web3LoginResponse>('/auth/web3/login/verify', request)
  return data
}

export async function verifyWeb3Registration(
  request: Web3RegistrationVerifyRequest,
): Promise<AuthResponse> {
  const { data } = await apiClient.post<AuthResponse>('/auth/web3/register/verify', request)
  return data
}
