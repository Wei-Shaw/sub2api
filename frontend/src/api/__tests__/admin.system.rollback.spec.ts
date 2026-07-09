import { beforeEach, describe, expect, it, vi REDACTED from 'vitest'

const { get, post REDACTED = vi.hoisted(() => ({
  get: vi.fn(),
  post: vi.fn(),
REDACTED))

vi.mock('../client', () => ({
  apiClient: {
    get,
    post,
  REDACTED,
REDACTED))

import { getRollbackVersions, rollback, type RollbackVersionInfo REDACTED from '@/api/admin/system'

describe('admin system rollback API', () => {
  beforeEach(() => {
    get.mockReset()
    post.mockReset()
  REDACTED)

  it('getRollbackVersions fetches the rollback version list', async () => {
    const versions: RollbackVersionInfo[] = [
      {
        version: '0.1.146',
        published_at: '2026-07-07T00:00:00Z',
        html_url: 'https://github.com/Wei-Shaw/sub2api/releases/tag/v0.1.146'
      REDACTED
    ]
    get.mockResolvedValue({ data: { versions REDACTED REDACTED)

    const result = await getRollbackVersions()

    expect(get).toHaveBeenCalledWith('/admin/system/rollback-versions')
    expect(result.versions).toEqual(versions)
  REDACTED)

  it('rollback posts the target version in the request body', async () => {
    post.mockResolvedValue({ data: { message: 'ok', need_restart: true REDACTED REDACTED)

    const result = await rollback('0.1.146')

    expect(post).toHaveBeenCalledWith('/admin/system/rollback', { version: '0.1.146' REDACTED)
    expect(result.need_restart).toBe(true)
  REDACTED)

  it('rollback without a version posts no body (legacy backup rollback)', async () => {
    post.mockResolvedValue({ data: { message: 'ok', need_restart: true REDACTED REDACTED)

    await rollback()

    expect(post).toHaveBeenCalledWith('/admin/system/rollback', undefined)
  REDACTED)
REDACTED)
