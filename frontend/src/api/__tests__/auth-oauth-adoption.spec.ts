import { beforeEach, describe, expect, it, vi REDACTED from 'vitest'

const post = vi.fn()

vi.mock('@/api/client', () => ({
  apiClient: {
    post
  REDACTED
REDACTED))

describe('oauth adoption auth api', () => {
  beforeEach(() => {
    post.mockReset()
    post.mockResolvedValue({ data: {REDACTED REDACTED)
    localStorage.clear()
    document.cookie = 'oauth_bind_access_token=; Max-Age=0; path=/'
  REDACTED)

  it('posts adoption decisions when exchanging pending oauth completion', async () => {
    const { exchangePendingOAuthCompletion REDACTED = await import('@/api/auth')

    await exchangePendingOAuthCompletion({
      adoptDisplayName: false,
      adoptAvatar: true
    REDACTED)

    expect(post).toHaveBeenCalledWith('/auth/oauth/pending/exchange', {
      adopt_display_name: false,
      adopt_avatar: true
    REDACTED)
  REDACTED)

  it('posts bind-login decisions when finalizing pending oauth bind flow', async () => {
    const { completePendingOAuthBindLogin REDACTED = await import('@/api/auth')

    await completePendingOAuthBindLogin({
      adoptDisplayName: true,
      adoptAvatar: false
    REDACTED)

    expect(post).toHaveBeenCalledWith('/auth/oauth/pending/exchange', {
      adopt_display_name: true,
      adopt_avatar: false
    REDACTED)
  REDACTED)

  it('posts linuxdo invitation completion with adoption decisions', async () => {
    const { completeLinuxDoOAuthRegistration REDACTED = await import('@/api/auth')

    await completeLinuxDoOAuthRegistration('invite-code', {
      adoptDisplayName: true,
      adoptAvatar: false
    REDACTED)

    expect(post).toHaveBeenCalledWith('/auth/oauth/linuxdo/complete-registration', {
      invitation_code: 'invite-code',
      adopt_display_name: true,
      adopt_avatar: false
    REDACTED)
  REDACTED)

  it('posts linuxdo create-account completion with adoption decisions', async () => {
    const { createPendingLinuxDoOAuthAccount REDACTED = await import('@/api/auth')

    await createPendingLinuxDoOAuthAccount('invite-code', {
      adoptDisplayName: false,
      adoptAvatar: true
    REDACTED)

    expect(post).toHaveBeenCalledWith('/auth/oauth/linuxdo/complete-registration', {
      invitation_code: 'invite-code',
      adopt_display_name: false,
      adopt_avatar: true
    REDACTED)
  REDACTED)

  it('posts affiliate code when completing linuxdo oauth registration', async () => {
    const { completeLinuxDoOAuthRegistration REDACTED = await import('@/api/auth')

    await completeLinuxDoOAuthRegistration(
      'invite-code',
      {
        adoptDisplayName: true,
        adoptAvatar: false
      REDACTED,
      ' AFF123 '
    )

    expect(post).toHaveBeenCalledWith('/auth/oauth/linuxdo/complete-registration', {
      invitation_code: 'invite-code',
      aff_code: 'AFF123',
      adopt_display_name: true,
      adopt_avatar: false
    REDACTED)
  REDACTED)

  it('posts oidc invitation completion with adoption decisions', async () => {
    const { completeOIDCOAuthRegistration REDACTED = await import('@/api/auth')

    await completeOIDCOAuthRegistration('invite-code', {
      adoptDisplayName: false,
      adoptAvatar: true
    REDACTED)

    expect(post).toHaveBeenCalledWith('/auth/oauth/oidc/complete-registration', {
      invitation_code: 'invite-code',
      adopt_display_name: false,
      adopt_avatar: true
    REDACTED)
  REDACTED)

  it('posts oidc create-account completion with adoption decisions', async () => {
    const { createPendingOIDCOAuthAccount REDACTED = await import('@/api/auth')

    await createPendingOIDCOAuthAccount('invite-code', {
      adoptDisplayName: true,
      adoptAvatar: false
    REDACTED)

    expect(post).toHaveBeenCalledWith('/auth/oauth/oidc/complete-registration', {
      invitation_code: 'invite-code',
      adopt_display_name: true,
      adopt_avatar: false
    REDACTED)
  REDACTED)

  it('posts wechat invitation completion with adoption decisions', async () => {
    const { completeWeChatOAuthRegistration REDACTED = await import('@/api/auth')

    await completeWeChatOAuthRegistration('invite-code', {
      adoptDisplayName: true,
      adoptAvatar: true
    REDACTED)

    expect(post).toHaveBeenCalledWith('/auth/oauth/wechat/complete-registration', {
      invitation_code: 'invite-code',
      adopt_display_name: true,
      adopt_avatar: true
    REDACTED)
  REDACTED)

  it('posts wechat create-account completion with adoption decisions', async () => {
    const { createPendingWeChatOAuthAccount REDACTED = await import('@/api/auth')

    await createPendingWeChatOAuthAccount('invite-code', {
      adoptDisplayName: false,
      adoptAvatar: false
    REDACTED)

    expect(post).toHaveBeenCalledWith('/auth/oauth/wechat/complete-registration', {
      invitation_code: 'invite-code',
      adopt_display_name: false,
      adopt_avatar: false
    REDACTED)
  REDACTED)

  it('posts affiliate code when creating pending wechat oauth account', async () => {
    const { createPendingWeChatOAuthAccount REDACTED = await import('@/api/auth')

    await createPendingWeChatOAuthAccount(
      'invite-code',
      {
        adoptDisplayName: false,
        adoptAvatar: true
      REDACTED,
      'WXAFF'
    )

    expect(post).toHaveBeenCalledWith('/auth/oauth/wechat/complete-registration', {
      invitation_code: 'invite-code',
      aff_code: 'WXAFF',
      adopt_display_name: false,
      adopt_avatar: true
    REDACTED)
  REDACTED)

  it('classifies oauth completion results as login or bind', async () => {
    const { getOAuthCompletionKind REDACTED = await import('@/api/auth')

    expect(getOAuthCompletionKind({ access_token: 'access-token' REDACTED)).toBe('login')
    expect(getOAuthCompletionKind({ redirect: '/profile' REDACTED)).toBe('bind')
  REDACTED)

  it('provides bind-login utility helpers for invitation and suggested profile states', async () => {
    const {
      getPendingOAuthBindLoginKind,
      hasPendingOAuthSuggestedProfile,
      isPendingOAuthCreateAccountRequired
    REDACTED = await import('@/api/auth')

    expect(getPendingOAuthBindLoginKind({ access_token: 'access-token' REDACTED)).toBe('login')
    expect(getPendingOAuthBindLoginKind({ redirect: '/profile' REDACTED)).toBe('bind')
    expect(
      isPendingOAuthCreateAccountRequired({
        error: 'invitation_required'
      REDACTED)
    ).toBe(true)
    expect(
      isPendingOAuthCreateAccountRequired({
        error: 'other'
      REDACTED)
    ).toBe(false)
    expect(
      hasPendingOAuthSuggestedProfile({
        suggested_display_name: 'OAuth Nick'
      REDACTED)
    ).toBe(true)
    expect(
      hasPendingOAuthSuggestedProfile({
        suggested_avatar_url: 'https://cdn.example/avatar.png'
      REDACTED)
    ).toBe(true)
    expect(hasPendingOAuthSuggestedProfile({REDACTED)).toBe(false)
  REDACTED)

  it('requests an HttpOnly oauth bind cookie before redirect binding', async () => {
    localStorage.setItem('auth_token', 'access-token-value')
    const { prepareOAuthBindAccessTokenCookie REDACTED = await import('@/api/auth')

    await prepareOAuthBindAccessTokenCookie()

    expect(post).toHaveBeenCalledWith('/auth/oauth/bind-token')
  REDACTED)
REDACTED)
