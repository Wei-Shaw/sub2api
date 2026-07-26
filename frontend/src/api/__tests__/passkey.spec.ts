import { afterEach, beforeEach, describe, expect, it, vi REDACTED from 'vitest'

const { get, post, patch, remove, credentialGet REDACTED = vi.hoisted(() => ({
  get: vi.fn(),
  post: vi.fn(),
  patch: vi.fn(),
  remove: vi.fn(),
  credentialGet: vi.fn()
REDACTED))

vi.mock('@/api/client', () => ({
  apiClient: {
    get,
    post,
    patch,
    delete: remove
  REDACTED
REDACTED))

import { passkeyAPI REDACTED from '@/api/passkey'

class FakePublicKeyCredential {
  id = 'credential-id'
  rawId = Uint8Array.from([1, 2, 3]).buffer
  type = 'public-key'
  authenticatorAttachment = 'platform'
  response = {
    authenticatorData: Uint8Array.from([4, 5]).buffer,
    clientDataJSON: Uint8Array.from([6, 7]).buffer,
    signature: Uint8Array.from([8, 9]).buffer,
    userHandle: Uint8Array.from([10, 11]).buffer
  REDACTED

  getClientExtensionResults(): AuthenticationExtensionsClientOutputs {
    return {REDACTED
  REDACTED
REDACTED

describe('passkey api', () => {
  beforeEach(() => {
    get.mockReset()
    post.mockReset()
    patch.mockReset()
    remove.mockReset()
    credentialGet.mockReset()

    vi.stubGlobal('PublicKeyCredential', FakePublicKeyCredential)
    Object.defineProperty(window, 'PublicKeyCredential', {
      configurable: true,
      value: FakePublicKeyCredential
    REDACTED)
    Object.defineProperty(navigator, 'credentials', {
      configurable: true,
      value: { get: credentialGet REDACTED
    REDACTED)
  REDACTED)

  afterEach(() => {
    vi.unstubAllGlobals()
  REDACTED)

  it('converts assertion options and response bytes to WebAuthn JSON', async () => {
    post
      .mockResolvedValueOnce({
        data: {
          session_token: 'one-time-session',
          options: {
            publicKey: {
              challenge: 'AQID',
              rpId: 'sub2api.example.com',
              userVerification: 'required'
            REDACTED
          REDACTED
        REDACTED
      REDACTED)
      .mockResolvedValueOnce({
        data: {
          access_token: 'access',
          token_type: 'Bearer',
          user: { id: 1 REDACTED
        REDACTED
      REDACTED)
    credentialGet.mockResolvedValue(new FakePublicKeyCredential())

    await passkeyAPI.login()

    const request = credentialGet.mock.calls[0][0] as CredentialRequestOptions
    expect(Array.from(new Uint8Array(request.publicKey!.challenge))).toEqual([1, 2, 3])
    expect(request.publicKey!.userVerification).toBe('required')

    expect(post).toHaveBeenNthCalledWith(2, '/auth/passkey/login/finish', {
      session_token: 'one-time-session',
      credential: {
        id: 'credential-id',
        rawId: 'AQID',
        type: 'public-key',
        authenticatorAttachment: 'platform',
        clientExtensionResults: {REDACTED,
        response: {
          authenticatorData: 'BAU',
          clientDataJSON: 'Bgc',
          signature: 'CAk',
          userHandle: 'Cgs'
        REDACTED
      REDACTED
    REDACTED)
  REDACTED)
REDACTED)
