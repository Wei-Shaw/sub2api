import { afterEach, describe, expect, it, vi } from 'vitest'
import {
  buildCodexModelCatalogUrl,
  buildCodexModelsManifestUrl,
  fetchCodexModelsManifest
} from '../codex'

describe('Codex models API', () => {
  afterEach(() => {
    vi.unstubAllGlobals()
  })

  it('builds the authenticated Codex manifest endpoint from the public API base', () => {
    expect(buildCodexModelsManifestUrl('https://example.com/api/v1/')).toBe(
      'https://example.com/api/v1/models?client_version=0.158.0'
    )
  })

  it.each([
    ['https://example.com', 'https://example.com/v1/models'],
    ['https://example.com/api/v1/', 'https://example.com/api/v1/models'],
    ['', `${window.location.origin}/v1/models`]
  ])('builds a version-free provider catalog URL from %s', (baseUrl, expected) => {
    expect(buildCodexModelCatalogUrl(baseUrl)).toBe(expected)
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
      text: async () => JSON.stringify(manifest)
    })
    vi.stubGlobal('fetch', fetchMock)

    const result = await fetchCodexModelsManifest('https://example.com/v1', 'sk-user-test')

    expect(fetchMock).toHaveBeenCalledWith(
      'https://example.com/v1/models?client_version=0.158.0',
      expect.objectContaining({
        headers: {
          Accept: 'application/json',
          Authorization: 'Bearer sk-user-test'
        }
      })
    )
    expect(result.modelCount).toBe(2)
    expect(result.responseBytes).toBe(new TextEncoder().encode(JSON.stringify(manifest)).byteLength)
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
      text: async () => JSON.stringify({ object: 'list', data: [] })
    }))

    await expect(fetchCodexModelsManifest('https://example.com/v1', 'sk-user-test'))
      .rejects.toThrow('valid manifest')
  })

  it('measures UTF-8 response bytes before formatting the downloadable catalog', async () => {
    const text = '{"models":[{"slug":"test","description":"模型"}]}'
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue({ ok: true, text: async () => text }))

    const result = await fetchCodexModelsManifest('https://example.com', 'sk-test')

    expect(result.responseBytes).toBe(new TextEncoder().encode(text).byteLength)
    expect(result.responseBytes).toBeGreaterThan(text.length)
    expect(result.content).toBe(JSON.stringify(JSON.parse(text), null, 2))
  })
})
