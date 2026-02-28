import { describe, expect, it, vi REDACTED from 'vitest'

vi.mock('@/stores/app', () => ({
  useAppStore: () => ({
    showError: vi.fn()
  REDACTED)
REDACTED))

vi.mock('@/api/admin', () => ({
  adminAPI: {
    accounts: {
      generateAuthUrl: vi.fn(),
      exchangeCode: vi.fn(),
      refreshOpenAIToken: vi.fn(),
      validateSoraSessionToken: vi.fn()
    REDACTED
  REDACTED
REDACTED))

import { useOpenAIOAuth REDACTED from '@/composables/useOpenAIOAuth'

describe('useOpenAIOAuth.buildCredentials', () => {
  it('should keep client_id when token response contains it', () => {
    const oauth = useOpenAIOAuth({ platform: 'sora' REDACTED)
    const creds = oauth.buildCredentials({
      access_token: 'at',
      refresh_token: 'rt',
      client_id: 'app_sora_client',
      expires_at: 1700000000
    REDACTED)

    expect(creds.client_id).toBe('app_sora_client')
    expect(creds.access_token).toBe('at')
    expect(creds.refresh_token).toBe('rt')
  REDACTED)

  it('should keep legacy behavior when client_id is missing', () => {
    const oauth = useOpenAIOAuth({ platform: 'openai' REDACTED)
    const creds = oauth.buildCredentials({
      access_token: 'at',
      refresh_token: 'rt',
      expires_at: 1700000000
    REDACTED)

    expect(Object.prototype.hasOwnProperty.call(creds, 'client_id')).toBe(false)
    expect(creds.access_token).toBe('at')
    expect(creds.refresh_token).toBe('rt')
  REDACTED)
REDACTED)
