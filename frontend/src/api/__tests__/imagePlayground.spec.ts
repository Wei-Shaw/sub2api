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
  deleteAdminImagePlaygroundImage,
  deleteAdminImagePlaygroundTask,
  deleteImagePlaygroundImage,
  downloadImagePlaygroundImage,
  deleteImagePlaygroundTask,
  getAdminImagePlaygroundPreview,
  getImagePlaygroundImagePreview,
  getImagePlaygroundOptions,
  getImagePlaygroundTask,
  listImagePlaygroundTasks,
  listAdminImagePlaygroundTasks,
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

  it('deletes individual images and exposes admin image management endpoints', async () => {
    const updatedTask = { id: 'task/one', status: 'completed', images: [{ index: 0 }] }
    const adminPage = { tasks: [], page: 1, page_size: 24, total: 0, total_images: 0, storage_bytes: 0 }
    const blob = new Blob(['image'], { type: 'image/png' })
    del.mockResolvedValueOnce({ status: 200, data: updatedTask })
    get.mockResolvedValueOnce({ data: adminPage })
    get.mockResolvedValueOnce({ data: blob })
    del.mockResolvedValueOnce({ status: 204 })
    del.mockResolvedValueOnce({ status: 204 })

    await expect(deleteImagePlaygroundImage('task/one', 1)).resolves.toBe(updatedTask)
    await expect(listAdminImagePlaygroundTasks(2, 50)).resolves.toBe(adminPage)
    await expect(getAdminImagePlaygroundPreview('task/one', 0)).resolves.toBe(blob)
    await expect(deleteAdminImagePlaygroundImage('task/one', 0)).resolves.toBeNull()
    await expect(deleteAdminImagePlaygroundTask('task/one')).resolves.toBeUndefined()

    expect(del).toHaveBeenNthCalledWith(1, '/image-playground/tasks/task%2Fone/images/1')
    expect(get).toHaveBeenNthCalledWith(1, '/admin/image-playground/tasks', { params: { page: 2, page_size: 50 } })
    expect(get).toHaveBeenNthCalledWith(2, '/admin/image-playground/tasks/task%2Fone/images/0', { responseType: 'blob', timeout: 60000 })
    expect(del).toHaveBeenNthCalledWith(2, '/admin/image-playground/tasks/task%2Fone/images/0')
    expect(del).toHaveBeenNthCalledWith(3, '/admin/image-playground/tasks/task%2Fone')
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
