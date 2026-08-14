import { beforeEach, describe, expect, it, vi REDACTED from 'vitest'

const { get REDACTED = vi.hoisted(() => ({
  get: vi.fn(),
REDACTED))

vi.mock('@/api/client', () => ({
  apiClient: { get REDACTED,
REDACTED))

import { getUsageSummary REDACTED from '@/api/admin/groups'

describe('admin group usage summary API', () => {
  beforeEach(() => {
    get.mockReset()
    get.mockResolvedValue({ data: [] REDACTED)
  REDACTED)

  it('does not send browser timezone parameters', async () => {
    const summary = [
      { group_id: 1, today_cost: 1.25, yesterday_cost: 2.5, total_cost: 9.75 REDACTED,
    ]
    get.mockResolvedValue({ data: summary REDACTED)

    await expect(getUsageSummary()).resolves.toEqual(summary)

    expect(get).toHaveBeenCalledWith('/admin/groups/usage-summary')
  REDACTED)
REDACTED)
