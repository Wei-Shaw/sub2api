import { afterEach, describe, expect, it, vi } from 'vitest'
import {
  buildCodexModelsManifestUrl,
  fetchCodexModelsManifest
} from '../codex'

describe('Codex models API', () => {
  afterEach(() => {
    vi.unstubAllGlobals()
  })

  it('builds the authenticated Codex manifest endpoint from the public API base', () => {
    expect(buildCodexModelsManifestUrl('https://example.com/api/v1/')).toBe(
      'https://example.com/api/v1/models?client_version=0.147.0'
    )
  })

  it('fetches a manifest with the current API key without adding it to the catalog', async () => {
    const manifest = {
      models: [
        {
          slug: 'grok-4.6',
          default_reasoning_level: 'high',
          supported_reasoning_levels: [
            { effort: 'low', description: 'Fast responses' },
            { effort: 'xhigh', description: 'Extra-high reasoning depth' }
          ],
          input_modalities: ['text', 'image'],
          model_messages: { instructions_template: 'Use the routed model.' }
        },
        {
          slug: 'deepseek-v4-pro',
          default_reasoning_level: 'high',
          supported_reasoning_levels: [
            { effort: 'low', description: 'Fast responses' },
            { effort: 'max', description: 'Maximum reasoning depth' }
          ],
          input_modalities: ['text'],
          model_messages: { instructions_template: 'Use the routed model.' }
        }
      ]
    }
    const fetchMock = vi.fn().mockResolvedValue({
      ok: true,
      status: 200,
      json: async () => manifest
    })
    vi.stubGlobal('fetch', fetchMock)

    const result = await fetchCodexModelsManifest('https://example.com/v1', 'sk-user-test')

    expect(fetchMock).toHaveBeenCalledWith(
      'https://example.com/v1/models?client_version=0.147.0',
      expect.objectContaining({
        headers: {
          Accept: 'application/json',
          Authorization: 'Bearer sk-user-test'
        }
      })
    )
    expect(result.modelCount).toBe(2)
    expect(JSON.parse(result.content)).toEqual(manifest)
    expect(result.content).toContain('"effort": "xhigh"')
    expect(result.content).toContain('"input_modalities"')
    expect(result.content).toContain('"instructions_template"')
    expect(result.content).not.toContain('sk-user-test')
  })

  it('rejects a successful response that is not a Codex manifest', async () => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue({
      ok: true,
      status: 200,
      json: async () => ({ object: 'list', data: [] })
    }))

    await expect(fetchCodexModelsManifest('https://example.com/v1', 'sk-user-test'))
      .rejects.toThrow('valid manifest')
  })
})
