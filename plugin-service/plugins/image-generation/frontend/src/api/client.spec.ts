import { describe, expect, it, vi } from 'vitest'
import { authenticatedMediaUrl, createPluginApi, loadImageGenerationPreference, saveImageGenerationPreference } from './client'
import type { GenerateRequest } from '../types'

function jsonResponse(body: unknown, status = 200): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { 'Content-Type': 'application/json' },
  })
}

describe('plugin api client', () => {

  it('loads and saves the gateway image-generation preference with the login token', async () => {
    window.localStorage.setItem('auth_token', 'preference-token')
    const fetchSpy = vi.fn<typeof fetch>()
      .mockResolvedValueOnce(jsonResponse({ code: 0, data: { last_api_key_id: 41 } }))
      .mockResolvedValueOnce(jsonResponse({ code: 0, data: { last_api_key_id: null } }))

    await expect(loadImageGenerationPreference(fetchSpy)).resolves.toEqual({ last_api_key_id: 41 })
    await expect(saveImageGenerationPreference(null, fetchSpy)).resolves.toEqual({ last_api_key_id: null })

    expect(fetchSpy).toHaveBeenNthCalledWith(1, '/api/v1/user/preferences/image-generation', expect.objectContaining({
      credentials: 'same-origin', headers: { Authorization: 'Bearer preference-token' },
    }))
    expect(fetchSpy).toHaveBeenNthCalledWith(2, '/api/v1/user/preferences/image-generation', expect.objectContaining({
      method: 'PUT', body: JSON.stringify({ last_api_key_id: null }), headers: expect.objectContaining({ Authorization: 'Bearer preference-token' }),
    }))
    window.localStorage.removeItem('auth_token')
  })
  it('loads image model capabilities from plugin config', async () => {
    const payload = { image_model_capabilities: { 'gpt-image-2': { max_reference_images: 16 } } }
    const fetchSpy = vi.fn<typeof fetch>().mockResolvedValue(jsonResponse(payload))
    const client = createPluginApi('/plugins/image-generation/api', fetchSpy)

    await expect(client.getConfig()).resolves.toEqual(payload)
    expect(fetchSpy).toHaveBeenCalledWith(
      '/plugins/image-generation/api/config',
      expect.objectContaining({ credentials: 'same-origin' }),
    )
  })

  it('uploads a reference image as multipart form data', async () => {
    const fetchSpy = vi.fn<typeof fetch>().mockResolvedValue(jsonResponse({
      name: 'reference.png', mime_type: 'image/png',
      storage_key: 'image-generation/uploads/7/original.png',
      preview_storage_key: 'image-generation/uploads/7/preview.jpg',
      original_url: '/plugins/image-generation/api/uploads/upload-1/original',
      preview_url: '/plugins/image-generation/api/uploads/upload-1/preview',
    }, 201))
    const client = createPluginApi('/plugins/image-generation/api', fetchSpy)
    const file = new File(['png-bytes'], 'reference.png', { type: 'image/png' })

    await client.uploadReference(file)

    const [, init] = fetchSpy.mock.calls[0]
    expect(init?.method).toBe('POST')
    expect(init?.body).toBeInstanceOf(FormData)
    expect((init?.body as FormData).get('image')).toBe(file)
    expect(init?.headers).toEqual({})
  })

  it('uses separate cursor-paged conversation endpoints', async () => {
    const fetchSpy = vi.fn<typeof fetch>().mockImplementation(async () => jsonResponse({ items: [] }))
    const client = createPluginApi('/plugins/image-generation/api', fetchSpy)

    await client.listConversations('next cursor')
    await client.listConversationMessages('conversation/1', 'older cursor')

    expect(fetchSpy.mock.calls[0][0]).toContain('/conversations?limit=20&cursor=next%20cursor')
    expect(fetchSpy.mock.calls[1][0]).toContain('/conversations/conversation%2F1/messages?limit=20&before=older%20cursor')
  })

	it('adds the login token to media URLs used by image elements', () => {
		window.localStorage.setItem('auth_token', 'media-token')
		expect(authenticatedMediaUrl('/plugins/image-generation/api/assets/h1/reference/0'))
			.toBe('/plugins/image-generation/api/assets/h1/reference/0?token=media-token')
		window.localStorage.removeItem('auth_token')
	})
  it('submits generation through the configured plugin base', async () => {
    const fetchSpy = vi.fn<typeof fetch>().mockResolvedValue(jsonResponse({ job_id: 'job-1', status: 'pending' }, 201))
    const client = createPluginApi('/plugins/image-generation/api/', fetchSpy)
    const request = { prompt: 'lamp', model: 'gpt-image-2' } as GenerateRequest

    await client.generate(request)

    expect(fetchSpy).toHaveBeenCalledWith(
      '/plugins/image-generation/api/generate',
      expect.objectContaining({
        method: 'POST',
        credentials: 'same-origin',
        body: JSON.stringify(request),
      }),
    )
  })

  it('exposes task actions and conversation deletion', async () => {
    const fetchSpy = vi.fn<typeof fetch>().mockImplementation(async () => jsonResponse({ id: 'history-1', status: 'pending' }))
    const client = createPluginApi('/plugins/image-generation/api', fetchSpy)

    await client.getStatus('history-1')
    await client.cancel('history-1')
    await client.deleteConversation('conversation-1')

    expect(fetchSpy).toHaveBeenNthCalledWith(1, '/plugins/image-generation/api/history/history-1/status', expect.objectContaining({ credentials: 'same-origin' }))
    expect(fetchSpy).toHaveBeenNthCalledWith(2, '/plugins/image-generation/api/history/history-1/cancel', expect.objectContaining({ method: 'POST' }))
    expect(fetchSpy).toHaveBeenNthCalledWith(3, '/plugins/image-generation/api/conversations/conversation-1', expect.objectContaining({ method: 'DELETE' }))
  })

  it('uses a structured API error message', async () => {
    const fetchSpy = vi.fn<typeof fetch>().mockResolvedValue(jsonResponse({ error: 'provider unavailable' }, 502))
    const client = createPluginApi('/plugins/image-generation/api', fetchSpy)

    await expect(client.listConversations()).rejects.toThrow('provider unavailable')
  })
})
