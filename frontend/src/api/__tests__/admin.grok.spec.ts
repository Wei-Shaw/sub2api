import { beforeEach, describe, expect, it, vi REDACTED from 'vitest'

const { post REDACTED = vi.hoisted(() => ({
  post: vi.fn(),
REDACTED))

vi.mock('@/api/client', () => ({
  apiClient: { post REDACTED,
REDACTED))

import { createFromSSO, getGrokSSOImportTimeout REDACTED from '@/api/admin/grok'

describe('admin Grok SSO import API', () => {
  beforeEach(() => {
    post.mockReset()
    post.mockResolvedValue({ data: { created: [], failed: [] REDACTED REDACTED)
  REDACTED)

  it.each([
    [1, 180_000],
    [3, 180_000],
    [4, 270_000],
    [7, 360_000],
  ])('uses a timeout sized for %i keys', async (keyCount, expectedTimeout) => {
    expect(getGrokSSOImportTimeout(keyCount)).toBe(expectedTimeout)

    await createFromSSO({
      sso_tokens: Array.from({ length: keyCount REDACTED, (_, index) => `sso-${index + 1REDACTED`),
    REDACTED)

    expect(post).toHaveBeenCalledWith(
      '/admin/grok/sso-to-oauth',
      expect.objectContaining({ sso_tokens: expect.any(Array) REDACTED),
      { timeout: expectedTimeout REDACTED,
    )
  REDACTED)
REDACTED)
