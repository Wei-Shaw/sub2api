import { beforeEach, describe, expect, it, vi } from 'vitest'

const post = vi.hoisted(() => vi.fn())

vi.mock('@/api/client', () => ({
  apiClient: { post },
}))

import { editImage, generateImage } from '../images'

describe('images API', () => {
  beforeEach(() => {
    post.mockReset()
  })

  it('posts generations through the /api/v1 client so the dev proxy handles the request', async () => {
    post.mockResolvedValueOnce({ data: { data: [] } })

    await generateImage({
      model: 'gpt-image-2',
      prompt: 'draw a cat',
      size: '1024x1024',
      quality: 'auto',
    }, 'sk-test')

    expect(post).toHaveBeenCalledWith('/images/generations', expect.any(Object), {
      headers: { Authorization: 'Bearer sk-test' },
      timeout: 0,
    })
  })

  it('posts edits through the /api/v1 client so the dev proxy handles the request', async () => {
    post.mockResolvedValueOnce({ data: { data: [] } })

    await editImage({
      model: 'gpt-image-2',
      prompt: 'make it cinematic',
      size: '1024x1024',
      quality: 'auto',
      images: [new File(['image'], 'ref.png', { type: 'image/png' })],
    }, 'sk-test')

    expect(post).toHaveBeenCalledWith('/images/edits', expect.any(FormData), {
      headers: { Authorization: 'Bearer sk-test' },
      timeout: 0,
    })
  })

})
