import { beforeEach, describe, expect, it, vi REDACTED from 'vitest'

const { post REDACTED = vi.hoisted(() => ({
  post: vi.fn(),
REDACTED))

vi.mock('@/api/client', () => ({
  apiClient: {
    post,
  REDACTED,
REDACTED))

import {
  bindUserAuthIdentity,
  type AdminBindAuthIdentityRequest,
  type AdminBoundAuthIdentity,
REDACTED from '@/api/admin/users'

type Assert<T extends true> = T
type IsExact<T, U> = (
  (<G>() => G extends T ? 1 : 2) extends (<G>() => G extends U ? 1 : 2)
    ? ((<G>() => G extends U ? 1 : 2) extends (<G>() => G extends T ? 1 : 2) ? true : false)
    : false
)

type ExpectedAdminBindAuthIdentityRequest = {
  provider_type: string
  provider_key: string
  provider_subject: string
  issuer?: string
  metadata?: Record<string, unknown>
  channel?: {
    channel: string
    channel_app_id: string
    channel_subject: string
    metadata?: Record<string, unknown>
  REDACTED
REDACTED

type ExpectedAdminBoundAuthIdentity = {
  user_id: number
  provider_type: string
  provider_key: string
  provider_subject: string
  verified_at?: string | null
  issuer?: string | null
  metadata: Record<string, unknown> | null
  created_at: string
  updated_at: string
  channel?: {
    channel: string
    channel_app_id: string
    channel_subject: string
    metadata: Record<string, unknown> | null
    created_at: string
    updated_at: string
  REDACTED | null
REDACTED

const requestContractExact: Assert<
  IsExact<AdminBindAuthIdentityRequest, ExpectedAdminBindAuthIdentityRequest>
> = true
const responseContractExact: Assert<
  IsExact<AdminBoundAuthIdentity, ExpectedAdminBoundAuthIdentity>
> = true

describe('admin users api auth identity binding', () => {
  beforeEach(() => {
    post.mockReset()
  REDACTED)

  it('posts the backend-compatible auth identity bind payload and returns the backend response shape', async () => {
    const payload: AdminBindAuthIdentityRequest = {
      provider_type: 'wechat',
      provider_key: 'wechat-main',
      provider_subject: 'union-123',
      metadata: { source: 'admin-repair' REDACTED,
      channel: {
        channel: 'open',
        channel_app_id: 'wx-open',
        channel_subject: 'openid-123',
        metadata: { scene: 'migration' REDACTED,
      REDACTED,
    REDACTED

    const response: AdminBoundAuthIdentity = {
      user_id: 9,
      provider_type: 'wechat',
      provider_key: 'wechat-main',
      provider_subject: 'union-123',
      verified_at: '2026-04-22T00:00:00Z',
      issuer: null,
      metadata: { source: 'admin-repair' REDACTED,
      created_at: '2026-04-22T00:00:00Z',
      updated_at: '2026-04-22T00:00:00Z',
      channel: {
        channel: 'open',
        channel_app_id: 'wx-open',
        channel_subject: 'openid-123',
        metadata: { scene: 'migration' REDACTED,
        created_at: '2026-04-22T00:00:00Z',
        updated_at: '2026-04-22T00:00:00Z',
      REDACTED,
    REDACTED
    post.mockResolvedValue({ data: response REDACTED)

    const result = await bindUserAuthIdentity(9, payload)

    expect(post).toHaveBeenCalledWith('/admin/users/9/auth-identities', payload)
    expect(result).toEqual(response)
  REDACTED)

  it('keeps bind auth identity request and response types aligned with the backend contract', () => {
    expect(requestContractExact).toBe(true)
    expect(responseContractExact).toBe(true)
  REDACTED)
REDACTED)
