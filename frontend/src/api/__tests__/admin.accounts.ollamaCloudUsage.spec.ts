import { beforeEach, describe, expect, it, vi REDACTED from 'vitest'

const { get, post, put, del REDACTED = vi.hoisted(() => ({
  get: vi.fn(),
  post: vi.fn(),
  put: vi.fn(),
  del: vi.fn()
REDACTED))

vi.mock('@/api/client', () => ({
  apiClient: { get, post, put, delete: del REDACTED
REDACTED))

import {
  deleteOllamaCloudUsageSession,
  getOllamaCloudUsage,
  getOllamaCloudUsageSettings,
  refreshOllamaCloudUsage,
  saveOllamaCloudUsageSession,
  setOllamaCloudUsageAutoRefresh,
  updateOllamaCloudUsageSettings
REDACTED from '@/api/admin/accounts'

const state = {
  account_id: 7,
  eligible: true,
  configured: true,
  auto_refresh_enabled: false,
  encryption_key_configured: true
REDACTED

describe('admin Ollama Cloud usage API', () => {
  beforeEach(() => {
    get.mockReset()
    post.mockReset()
    put.mockReset()
    del.mockReset()
  REDACTED)

  it('uses dedicated global settings endpoints', async () => {
    const settings = { enabled: false, interval_minutes: 60, debounce_minutes: 1 REDACTED
    get.mockResolvedValueOnce({ data: settings REDACTED)
    put.mockResolvedValueOnce({ data: settings REDACTED)

    await expect(getOllamaCloudUsageSettings()).resolves.toEqual(settings)
    await expect(updateOllamaCloudUsageSettings(settings)).resolves.toEqual(settings)
    expect(get).toHaveBeenCalledWith('/admin/accounts/ollama-cloud-usage/settings')
    expect(put).toHaveBeenCalledWith('/admin/accounts/ollama-cloud-usage/settings', settings)
  REDACTED)

  it('keeps session configuration write-only and separate from account updates', async () => {
    get.mockResolvedValueOnce({ data: state REDACTED)
    put.mockResolvedValueOnce({ data: state REDACTED).mockResolvedValueOnce({ data: state REDACTED)
    del.mockResolvedValueOnce({ data: { ...state, configured: false REDACTED REDACTED)
    post.mockResolvedValueOnce({ data: state REDACTED)

    await expect(getOllamaCloudUsage(7)).resolves.toEqual(state)
    await expect(saveOllamaCloudUsageSession(7, 'wos-session=secret')).resolves.toEqual(state)
    await expect(setOllamaCloudUsageAutoRefresh(7, true)).resolves.toEqual(state)
    await expect(refreshOllamaCloudUsage(7)).resolves.toEqual(state)
    await expect(deleteOllamaCloudUsageSession(7)).resolves.toMatchObject({ configured: false REDACTED)

    expect(put).toHaveBeenNthCalledWith(1, '/admin/accounts/7/ollama-cloud-usage/session', { session: 'wos-session=secret' REDACTED)
    expect(put).toHaveBeenNthCalledWith(2, '/admin/accounts/7/ollama-cloud-usage/auto-refresh', { enabled: true REDACTED)
    expect(post).toHaveBeenCalledWith('/admin/accounts/7/ollama-cloud-usage/refresh')
    expect(del).toHaveBeenCalledWith('/admin/accounts/7/ollama-cloud-usage/session')
  REDACTED)
REDACTED)
