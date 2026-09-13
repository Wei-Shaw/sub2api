import { afterEach, describe, expect, it, vi } from 'vitest'

import { listModels } from '../keys'

describe('keys API model list', () => {
  afterEach(() => {
    vi.unstubAllGlobals()
  })

  it('uses the selected API key and normalizes the effective model list', async () => {
    const fetchMock = vi.fn().mockResolvedValue({
      ok: true,
      status: 200,
      json: vi.fn().mockResolvedValue({
        object: 'list',
        data: [
          { id: 'gpt-5.6', display_name: 'GPT-5.6' },
          { id: ' gpt-5-mini ' },
          { id: 'gpt-5.6' },
          { display_name: 'missing-id' },
        ],
      }),
    })
    vi.stubGlobal('fetch', fetchMock)
    const controller = new AbortController()

    await expect(
      listModels('sk-test-key', { signal: controller.signal })
    ).resolves.toEqual([
      { id: 'gpt-5.6', display_name: 'GPT-5.6' },
      { id: 'gpt-5-mini', display_name: undefined },
    ])
    expect(fetchMock).toHaveBeenCalledWith(
      expect.stringMatching(/\/v1\/models$/),
      expect.objectContaining({
        credentials: 'omit',
        headers: {
          Accept: 'application/json',
          Authorization: 'Bearer sk-test-key',
        },
        signal: controller.signal,
      })
    )
  })

  it('rejects failed gateway responses', async () => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue({
      ok: false,
      status: 401,
    }))

    await expect(listModels('sk-disabled-key')).rejects.toThrow(
      'Failed to load models (401)'
    )
  })

  it('rejects malformed model responses', async () => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue({
      ok: true,
      status: 200,
      json: vi.fn().mockResolvedValue({ data: null }),
    }))

    await expect(listModels('sk-test-key')).rejects.toThrow(
      'Invalid models response'
    )
  })
})
