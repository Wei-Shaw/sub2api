import { beforeEach, describe, expect, it, vi REDACTED from 'vitest'

const { get, post, put REDACTED = vi.hoisted(() => ({
  get: vi.fn(),
  post: vi.fn(),
  put: vi.fn()
REDACTED))

vi.mock('@/api/client', () => ({
  apiClient: { get, post, put REDACTED
REDACTED))

import {
  getUpstreamBillingProbeSettings,
  probeUpstreamBilling,
  probeUpstreamBillingBatch,
  setUpstreamBillingProbeEnabled,
  updateUpstreamBillingProbeSettings
REDACTED from '@/api/admin/accounts'

describe('admin account upstream billing probe API', () => {
  beforeEach(() => {
    get.mockReset()
    post.mockReset()
    put.mockReset()
  REDACTED)

  it('reads and updates global settings', async () => {
    const settings = { enabled: true, interval_minutes: 30 REDACTED
    get.mockResolvedValueOnce({ data: settings REDACTED)
    put.mockResolvedValueOnce({ data: settings REDACTED)

    await expect(getUpstreamBillingProbeSettings()).resolves.toEqual(settings)
    await expect(updateUpstreamBillingProbeSettings(settings)).resolves.toEqual(settings)
    expect(get).toHaveBeenCalledWith('/admin/accounts/upstream-billing-probe/settings')
    expect(put).toHaveBeenCalledWith('/admin/accounts/upstream-billing-probe/settings', settings)
  REDACTED)

  it('uses dedicated account and batch endpoints', async () => {
    const result = { account_id: 7, snapshot: { status: 'unsupported' REDACTED REDACTED
    put.mockResolvedValueOnce({ data: {REDACTED REDACTED)
    post.mockResolvedValueOnce({ data: result REDACTED)
    post.mockResolvedValueOnce({ data: { results: [result] REDACTED REDACTED)

    await setUpstreamBillingProbeEnabled(7, true)
    await expect(probeUpstreamBilling(7)).resolves.toEqual(result)
    await expect(probeUpstreamBillingBatch([7])).resolves.toEqual([result])

    expect(put).toHaveBeenCalledWith('/admin/accounts/7/upstream-billing-probe', { enabled: true REDACTED)
    expect(post).toHaveBeenNthCalledWith(1, '/admin/accounts/7/upstream-billing-probe')
    expect(post).toHaveBeenNthCalledWith(2, '/admin/accounts/upstream-billing-probe/batch', { account_ids: [7] REDACTED)
  REDACTED)
REDACTED)
