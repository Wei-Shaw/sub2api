import { afterEach, describe, expect, it, vi REDACTED from 'vitest'
import { apiClient REDACTED from '../client'
import { getMatrix, repeatedArrayParamsSerializer REDACTED from '../channelMonitorV2'

afterEach(() => vi.restoreAllMocks())

describe('channel monitor V2 query serialization', () => {
  it('uses repeated keys without bracket suffixes for array filters', () => {
    const query = repeatedArrayParamsSerializer({
      range: '90m',
      platform: ['openai', 'grok'],
      group_id: [1, 2],
      model: undefined,
      group_by: 'platform_group_model',
    REDACTED)

    expect(query).toBe('range=90m&platform=openai&platform=grok&group_id=1&group_id=2&group_by=platform_group_model')
    expect(query).not.toContain('%5B%5D')
  REDACTED)

  it('sends the matrix grouping with the shared filters', async () => {
    const get = vi.spyOn(apiClient, 'get').mockResolvedValue({
      data: { coverage: {REDACTED, group_by: 'platform_group', items: [] REDACTED,
    REDACTED)

    await getMatrix({ range: '24h', platforms: ['openai'], groupIds: [7], models: [] REDACTED, 'platform_group', true)

    expect(get).toHaveBeenCalledWith('/admin/channel-monitor-v2/matrix', expect.objectContaining({
      params: {
        range: '24h',
        platform: ['openai'],
        group_id: [7],
        model: undefined,
        group_by: 'platform_group',
      REDACTED,
    REDACTED))
  REDACTED)
REDACTED)
