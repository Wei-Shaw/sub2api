import { beforeEach, describe, expect, it, vi } from 'vitest'

const { get, post, del } = vi.hoisted(() => ({
  get: vi.fn(),
  post: vi.fn(),
  del: vi.fn(),
}))

vi.mock('@/api/client', () => ({
  apiClient: { get, post, delete: del },
}))

import {
  downloadImagePlaygroundImage,
  deleteImagePlaygroundTask,
  getImagePlaygroundImagePreview,
  getImagePlaygroundOptions,
  getImagePlaygroundTask,
  listImagePlaygroundTasks,
  submitImagePlaygroundTask,
} from '@/api/imagePlayground'

describe('image playground api', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  it('loads options and submits generation tasks through the dashboard API', async () => {
    const options = { enabled: true, groups: [] }
    const task = { id: 'task-1', status: 'processing' }
    get.mockResolvedValueOnce({ data: options })
    post.mockResolvedValueOnce({ data: task })

    await expect(getImagePlaygroundOptions()).resolves.toBe(options)
    await expect(submitImagePlaygroundTask({
      group_id: 12,
      model: 'gpt-image-1.5',
      prompt: 'A quiet library',
      n: 1,
    })).resolves.toBe(task)

    expect(get).toHaveBeenCalledWith('/image-playground/options')
    expect(post).toHaveBeenCalledWith('/image-playground/tasks', {
      group_id: 12,
      model: 'gpt-image-1.5',
      prompt: 'A quiet library',
      n: 1,
    })
  })

  it('encodes task ids and requests previews and downloads as blobs', async () => {
    const task = { id: 'task/one', status: 'completed' }
    const blob = new Blob(['image'], { type: 'image/png' })
    get.mockResolvedValueOnce({ data: task })
    get.mockResolvedValueOnce({ data: blob })
    get.mockResolvedValueOnce({ data: blob })

    await expect(getImagePlaygroundTask('task/one')).resolves.toBe(task)
    await expect(getImagePlaygroundImagePreview('task/one', 1)).resolves.toBe(blob)
    await expect(downloadImagePlaygroundImage('task/one', 2)).resolves.toBe(blob)

    expect(get).toHaveBeenNthCalledWith(1, '/image-playground/tasks/task%2Fone')
    expect(get).toHaveBeenNthCalledWith(
      2,
      '/image-playground/tasks/task%2Fone/images/1',
      { responseType: 'blob', timeout: 60000 },
    )
    expect(get).toHaveBeenNthCalledWith(
      3,
      '/image-playground/tasks/task%2Fone/images/2/download',
      { responseType: 'blob', timeout: 60000 },
    )
  })

  it('lists the authenticated user image tasks', async () => {
    const tasks = [{ id: 'task-1', status: 'completed' }]
    get.mockResolvedValueOnce({ data: tasks })

    await expect(listImagePlaygroundTasks()).resolves.toBe(tasks)
    expect(get).toHaveBeenCalledWith('/image-playground/tasks')
  })

  it('deletes a task using an encoded id', async () => {
    del.mockResolvedValueOnce({})
    await expect(deleteImagePlaygroundTask('task/one')).resolves.toBeUndefined()
    expect(del).toHaveBeenCalledWith('/image-playground/tasks/task%2Fone')
  })

  it('submits reference images as multipart form data', async () => {
    const task = { id: 'task-edit', status: 'processing' }
    post.mockResolvedValueOnce({ data: task })
    const image = new File(['image-bytes'], 'reference.png', { type: 'image/png' })

    await expect(submitImagePlaygroundTask({
      group_id: 12,
      model: 'gpt-image-2',
      prompt: 'Keep the subject and change the background',
      n: 1,
    }, [image])).resolves.toBe(task)

    const [url, body, config] = post.mock.calls[0]
    expect(url).toBe('/image-playground/tasks')
    expect(body).toBeInstanceOf(FormData)
    expect((body as FormData).get('group_id')).toBe('12')
    expect((body as FormData).get('model')).toBe('gpt-image-2')
    expect((body as FormData).getAll('images')).toEqual([image])
    expect(config).toEqual({
      timeout: 60000,
    })
  })
})
