import { beforeEach, describe, expect, it, vi } from 'vitest'

const { post, get } = vi.hoisted(() => ({ post: vi.fn(), get: vi.fn() }))

vi.mock('@/api/client', () => ({
  apiClient: { post, get },
}))

import { createImageEditTask, createImageGenerationTask, downloadImageTask, editImage, generateImage, getImageTask } from '../images'

describe('images API', () => {
  beforeEach(() => {
    post.mockReset()
    get.mockReset()
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

  it('creates async generation tasks without the long image timeout', async () => {
    post.mockResolvedValueOnce({ data: { task_id: 'img_123', status: 'pending', expires_at: '2026-05-08T12:00:00Z' } })

    await createImageGenerationTask({
      model: 'gpt-image-2',
      prompt: 'draw a cat',
      size: '1024x1024',
      quality: 'auto',
    }, 'sk-test')

    expect(post).toHaveBeenCalledWith('/images/async/generations', expect.any(Object), {
      headers: { Authorization: 'Bearer sk-test' },
    })
  })

  it('creates async edit tasks and polls task status', async () => {
    post.mockResolvedValueOnce({ data: { task_id: 'img_123', status: 'pending', expires_at: '2026-05-08T12:00:00Z' } })
    get.mockResolvedValueOnce({ data: { task_id: 'img_123', status: 'succeeded', download_url: '/download', expires_at: '2026-05-08T12:00:00Z' } })
    get.mockResolvedValueOnce({ data: new Blob(['image'], { type: 'image/png' }) })

    await createImageEditTask({
      model: 'gpt-image-2',
      prompt: 'make it cinematic',
      size: '1024x1024',
      quality: 'auto',
      images: [new File(['image'], 'ref.png', { type: 'image/png' })],
    }, 'sk-test')
    await getImageTask('img_123', 'sk-test')
    await downloadImageTask('img_123', 'sk-test')

    expect(post).toHaveBeenCalledWith('/images/async/edits', expect.any(FormData), {
      headers: { Authorization: 'Bearer sk-test' },
    })
    expect(get).toHaveBeenCalledWith('/images/async/tasks/img_123', {
      headers: { Authorization: 'Bearer sk-test' },
    })
    expect(get).toHaveBeenCalledWith('/images/async/tasks/img_123/download', {
      headers: { Authorization: 'Bearer sk-test' },
      responseType: 'blob',
    })
  })

})
