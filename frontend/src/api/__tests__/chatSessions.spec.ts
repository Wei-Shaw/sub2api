import { beforeEach, describe, expect, it, vi } from 'vitest'

const get = vi.hoisted(() => vi.fn())
const post = vi.hoisted(() => vi.fn())
const patch = vi.hoisted(() => vi.fn())
const del = vi.hoisted(() => vi.fn())

vi.mock('../client', () => ({
  apiClient: {
    get,
    post,
    patch,
    delete: del,
  },
}))

describe('chat session API', () => {
  beforeEach(() => {
    get.mockReset()
    post.mockReset()
    patch.mockReset()
    del.mockReset()
  })

  it('lists and creates chat sessions', async () => {
    get.mockResolvedValueOnce({ data: [{ id: 1, title: 'History' }] })
    post.mockResolvedValueOnce({ data: { id: 2, title: 'New chat' } })
    const { listChatSessions, createChatSession } = await import('../chatSessions')

    await expect(listChatSessions()).resolves.toEqual([{ id: 1, title: 'History' }])
    await createChatSession({ api_key_id: 9, title: 'New chat', model: 'gpt-5.4' })

    expect(get).toHaveBeenCalledWith('/chat/sessions')
    expect(post).toHaveBeenCalledWith('/chat/sessions', {
      api_key_id: 9,
      title: 'New chat',
      model: 'gpt-5.4',
    })
  })

  it('loads, creates, updates messages and deletes sessions', async () => {
    get.mockResolvedValueOnce({ data: [{ id: 10, role: 'user', content: 'hello' }] })
    post.mockResolvedValueOnce({ data: { id: 11, role: 'assistant', status: 'streaming' } })
    patch.mockResolvedValueOnce({ data: { id: 11, status: 'completed' } })
    del.mockResolvedValueOnce({ data: { deleted: true } })
    const {
      getChatSessionMessages,
      createChatMessage,
      updateChatMessage,
      deleteChatSession,
    } = await import('../chatSessions')

    await expect(getChatSessionMessages(7)).resolves.toEqual([{ id: 10, role: 'user', content: 'hello' }])
    await createChatMessage(7, { role: 'assistant', content: '', status: 'streaming' })
    await updateChatMessage(7, 11, { content: 'done', status: 'completed', duration_ms: 1200 })
    await deleteChatSession(7)

    expect(get).toHaveBeenCalledWith('/chat/sessions/7/messages')
    expect(post).toHaveBeenCalledWith('/chat/sessions/7/messages', {
      role: 'assistant',
      content: '',
      status: 'streaming',
    })
    expect(patch).toHaveBeenCalledWith('/chat/sessions/7/messages/11', {
      content: 'done',
      status: 'completed',
      duration_ms: 1200,
    })
    expect(del).toHaveBeenCalledWith('/chat/sessions/7')
  })

  it('updates session metadata', async () => {
    patch.mockResolvedValueOnce({ data: { id: 3, title: 'Renamed' } })
    const { updateChatSession } = await import('../chatSessions')

    await updateChatSession(3, { title: 'Renamed', model: 'gpt-5.4-mini' })

    expect(patch).toHaveBeenCalledWith('/chat/sessions/3', {
      title: 'Renamed',
      model: 'gpt-5.4-mini',
    })
  })
})
