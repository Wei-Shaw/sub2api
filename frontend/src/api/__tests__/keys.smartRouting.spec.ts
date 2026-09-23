import { afterEach, describe, expect, it, vi } from 'vitest'

import { apiClient } from '../client'
import { create, update } from '../keys'

afterEach(() => {
  vi.restoreAllMocks()
})

describe('API key smart routing payloads', () => {
  it('includes the ordered routing groups when creating a smart routing key', async () => {
    const post = vi.spyOn(apiClient, 'post').mockResolvedValue({ data: {} })

    await create('smart-key', 12, undefined, undefined, undefined, undefined, undefined, undefined, [12, 7, 3])

    expect(post).toHaveBeenCalledWith('/keys', {
      name: 'smart-key',
      group_id: 12,
      routing_group_ids: [12, 7, 3],
    })
  })

  it('sends an empty routing list when switching an existing key back to a fixed group', async () => {
    const put = vi.spyOn(apiClient, 'put').mockResolvedValue({ data: {} })

    await update(42, {
      name: 'fixed-key',
      group_id: 9,
      routing_group_ids: [],
    })

    expect(put).toHaveBeenCalledWith('/keys/42', {
      name: 'fixed-key',
      group_id: 9,
      routing_group_ids: [],
    })
  })
})
