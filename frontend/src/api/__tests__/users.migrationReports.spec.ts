import { beforeEach, describe, expect, it, vi REDACTED from 'vitest'

const { get, post REDACTED = vi.hoisted(() => ({
  get: vi.fn(),
  post: vi.fn(),
REDACTED))

vi.mock('@/api/client', () => ({
  apiClient: {
    get,
    post,
  REDACTED,
REDACTED))

import {
  getAuthIdentityMigrationReportSummary,
  listAuthIdentityMigrationReports,
  resolveAuthIdentityMigrationReport,
REDACTED from '@/api/admin/users'

describe('admin users auth identity migration reports API', () => {
  beforeEach(() => {
    get.mockReset()
    post.mockReset()
  REDACTED)

  it('lists migration reports with pagination and report type filter', async () => {
    const response = {
      items: [],
      total: 0,
      page: 2,
      page_size: 10,
      pages: 0,
    REDACTED
    get.mockResolvedValue({ data: response REDACTED)

    const result = await listAuthIdentityMigrationReports({
      page: 2,
      pageSize: 10,
      reportType: 'oidc_synthetic_email_requires_manual_recovery',
    REDACTED)

    expect(get).toHaveBeenCalledWith('/admin/users/auth-identity-migration-reports', {
      params: {
        page: 2,
        page_size: 10,
        report_type: 'oidc_synthetic_email_requires_manual_recovery',
      REDACTED,
    REDACTED)
    expect(result).toBe(response)
  REDACTED)

  it('loads migration report summary', async () => {
    const response = {
      total: 2,
      open_total: 1,
      resolved_total: 1,
      by_type: {
        oidc_synthetic_email_requires_manual_recovery: 2,
      REDACTED,
    REDACTED
    get.mockResolvedValue({ data: response REDACTED)

    const result = await getAuthIdentityMigrationReportSummary()

    expect(get).toHaveBeenCalledWith('/admin/users/auth-identity-migration-reports/summary')
    expect(result).toBe(response)
  REDACTED)

  it('submits report resolution note', async () => {
    const response = {
      id: 7,
      resolution_note: 'resolved by admin',
    REDACTED
    post.mockResolvedValue({ data: response REDACTED)

    const result = await resolveAuthIdentityMigrationReport(7, 'resolved by admin')

    expect(post).toHaveBeenCalledWith('/admin/users/auth-identity-migration-reports/7/resolve', {
      resolution_note: 'resolved by admin',
    REDACTED)
    expect(result).toBe(response)
  REDACTED)
REDACTED)
