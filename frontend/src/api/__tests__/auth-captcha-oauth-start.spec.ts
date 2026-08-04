import { beforeEach, describe, expect, it, vi REDACTED from 'vitest'

const post = vi.hoisted(() => vi.fn())

vi.mock('@/api/client', () => ({
  apiClient: { post REDACTED
REDACTED))

import {
  buildOAuthLoginStartURL,
  startOAuthLogin,
  type OAuthLoginStart
REDACTED from '@/api/auth'

describe('OAuth captcha start API', () => {
  beforeEach(() => {
    post.mockReset()
  REDACTED)

  it('posts Tencent captcha proof and preserves OAuth query parameters', async () => {
    const request: OAuthLoginStart = {
      provider: 'github',
      params: { redirect: '/dashboard', aff_code: 'AFF123' REDACTED
    REDACTED
    const proof = {
      tencent_captcha_ticket: 'ticket',
      tencent_captcha_randstr: '@rand'
    REDACTED
    post.mockResolvedValue({ data: { authorize_url: 'https://github.com/login/oauth/authorize' REDACTED REDACTED)

    await expect(startOAuthLogin(request, proof)).resolves.toEqual({
      authorize_url: 'https://github.com/login/oauth/authorize'
    REDACTED)
    expect(post).toHaveBeenCalledWith('/auth/oauth/github/start', proof, {
      params: request.params
    REDACTED)
  REDACTED)

  it('builds the legacy GET start URL when Tencent captcha is disabled', () => {
    expect(buildOAuthLoginStartURL({
      provider: 'wechat',
      params: { mode: 'open', redirect: '/billing?plan=pro' REDACTED
    REDACTED)).toBe('/api/v1/auth/oauth/wechat/start?mode=open&redirect=%2Fbilling%3Fplan%3Dpro')
  REDACTED)
REDACTED)
