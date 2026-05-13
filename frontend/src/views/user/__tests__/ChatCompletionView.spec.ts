import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'

const cachedPublicSettings = vi.hoisted(() => ({ value: { chat_completion_enabled: true, api_base_url: 'https://gateway.example.com' } }))
const showError = vi.hoisted(() => vi.fn())
const {
  listKeys,
  getAvailableChannels,
  streamChatCompletion,
  listChatSessions,
  createChatSession,
  getChatSessionMessages,
  createChatMessage,
  updateChatMessage,
  copyToClipboard,
} = vi.hoisted(() => ({
  listKeys: vi.fn(),
  getAvailableChannels: vi.fn(),
  streamChatCompletion: vi.fn(),
  listChatSessions: vi.fn(),
  createChatSession: vi.fn(),
  getChatSessionMessages: vi.fn(),
  createChatMessage: vi.fn(),
  updateChatMessage: vi.fn(),
  copyToClipboard: vi.fn(),
}))
const storage = vi.hoisted(() => new Map<string, string>())

vi.stubGlobal('localStorage', {
  getItem: vi.fn((key: string) => storage.get(key) ?? null),
  setItem: vi.fn((key: string, value: string) => {
    storage.set(key, value)
  }),
  removeItem: vi.fn((key: string) => {
    storage.delete(key)
  }),
  clear: vi.fn(() => {
    storage.clear()
  }),
})

vi.mock('@/api/keys', () => ({
  keysAPI: {
    list: listKeys,
  },
}))

vi.mock('@/api/channels', () => ({
  default: {
    getAvailable: getAvailableChannels,
  },
}))

vi.mock('@/api/chat', () => ({
  streamChatCompletion,
}))

vi.mock('@/api/chatSessions', () => ({
  listChatSessions,
  createChatSession,
  getChatSessionMessages,
  createChatMessage,
  updateChatMessage,
  deleteChatSession: vi.fn(),
}))

vi.mock('@/composables/useClipboard', () => ({
  useClipboard: () => ({ copyToClipboard }),
}))

vi.mock('@/components/layout/AppLayout.vue', () => ({
  default: { template: '<div><slot /></div>' },
}))

vi.mock('@/components/chat/MarkdownMessage.vue', () => ({
  default: { props: ['content'], template: '<div data-testid="markdown-message">{{ content }}</div>' },
}))

vi.mock('@/stores/app', () => ({
  useAppStore: () => ({
    cachedPublicSettings: cachedPublicSettings.value,
    showError,
  }),
}))

vi.mock('vue-i18n', () => ({
  useI18n: () => ({
    t: (key: string) => key,
  }),
}))

import ChatCompletionView from '../ChatCompletionView.vue'

describe('ChatCompletionView', () => {
  beforeEach(() => {
    localStorage.clear()
    cachedPublicSettings.value = { chat_completion_enabled: true, api_base_url: 'https://gateway.example.com' }
    showError.mockReset()
    listKeys.mockReset()
    getAvailableChannels.mockReset()
    streamChatCompletion.mockReset()
    listChatSessions.mockReset()
    createChatSession.mockReset()
    getChatSessionMessages.mockReset()
    createChatMessage.mockReset()
    updateChatMessage.mockReset()
    copyToClipboard.mockReset()
    listKeys.mockResolvedValue({
      items: [
        {
          id: 1,
          key: 'sk-active',
          name: 'Primary Key',
          status: 'active',
          expires_at: null,
          group_id: 10,
        },
      ],
    })
    getAvailableChannels.mockResolvedValue([
      {
        name: 'OpenAI',
        platforms: [
          {
            platform: 'openai',
            base_url: 'https://api.openai.com',
            groups: [{ id: 10, name: 'Default' }],
            supported_models: [
              { name: 'gpt-5.4', platform: 'openai' },
              { name: 'gpt-5.4-mini', platform: 'openai' },
              { name: 'gpt-image-2', platform: 'openai', pricing: { billing_mode: 'image' } },
              { name: 'dall-e-3', platform: 'openai' },
            ],
          },
        ],
      },
    ])
    listChatSessions.mockResolvedValue([])
    createChatSession.mockResolvedValue({
      id: 100,
      api_key_id: 1,
      title: 'hello',
      model: 'gpt-5.4',
      status: 'active',
      expires_at: '2026-06-12T00:00:00Z',
      created_at: '2026-05-13T00:00:00Z',
      updated_at: '2026-05-13T00:00:00Z',
    })
    createChatMessage.mockImplementation((_sessionId, payload) => Promise.resolve({
      id: payload.role === 'user' ? 201 : 202,
      session_id: 100,
      role: payload.role,
      content: payload.content,
      status: payload.status || 'completed',
      model: payload.model,
      created_at: '2026-05-13T00:00:00Z',
      updated_at: '2026-05-13T00:00:00Z',
    }))
    updateChatMessage.mockImplementation((_sessionId, messageId, payload) => Promise.resolve({
      id: messageId,
      session_id: 100,
      role: 'assistant',
      content: payload.content || '',
      status: payload.status || 'completed',
      model: payload.model,
      duration_ms: payload.duration_ms,
      actual_cost: payload.actual_cost,
      error_message: payload.error_message,
      created_at: '2026-05-13T00:00:00Z',
      updated_at: '2026-05-13T00:00:00Z',
    }))
    streamChatCompletion.mockResolvedValue(undefined)
  })

  it('renders the chat workbench when the feature is enabled', async () => {
    const wrapper = mount(ChatCompletionView)
    await flushPromises()

    expect(wrapper.find('[data-testid="chat-workbench"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="chat-toolbar"]').exists()).toBe(false)
    expect(wrapper.find('[data-testid="chat-message-list"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="chat-composer"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="chat-session-sidebar"]').exists()).toBe(true)
  })

  it('uses the same full-width page shell as other user pages', async () => {
    const wrapper = mount(ChatCompletionView)
    await flushPromises()

    const shell = wrapper.get('[data-testid="chat-page-shell"]')
    expect(shell.classes()).toContain('w-full')
    expect(shell.classes()).not.toContain('mx-auto')
    expect(shell.classes()).not.toContain('max-w-7xl')
  })

  it('keeps history primary and moves key and model controls into a settings drawer', async () => {
    const wrapper = mount(ChatCompletionView)
    await flushPromises()

    const sidebar = wrapper.get('[data-testid="chat-session-sidebar"]')
    expect(sidebar.find('[data-testid="chat-new-button"]').exists()).toBe(true)
    expect(sidebar.find('#chat-api-key').exists()).toBe(false)
    expect(sidebar.find('#chat-model').exists()).toBe(false)

    await sidebar.get('[data-testid="chat-settings-toggle"]').trigger('click')

    expect(sidebar.text()).toContain('chatCompletion.chatSettings')
    expect(sidebar.text()).toContain('chatCompletion.baseUrl')
    expect(sidebar.text()).toContain('https://gateway.example.com')
    expect(sidebar.text()).not.toContain('https://api.openai.com')
    expect(sidebar.text()).toContain('chatCompletion.apiKey')
    expect(sidebar.text()).toContain('chatCompletion.model')
    expect(sidebar.find('#chat-api-key').exists()).toBe(true)
    expect(sidebar.find('#chat-model').exists()).toBe(true)
    expect(wrapper.find('[data-testid="chat-toolbar"]').exists()).toBe(false)
  })

  it('filters image generation models out of the chat model selector', async () => {
    const wrapper = mount(ChatCompletionView)
    await flushPromises()
    await wrapper.get('[data-testid="chat-settings-toggle"]').trigger('click')

    const options = wrapper.findAll('#chat-model option').map((option) => option.text())
    expect(options).toContain('gpt-5.4')
    expect(options).toContain('gpt-5.4-mini')
    expect(options).not.toContain('gpt-image-2')
    expect(options).not.toContain('dall-e-3')
  })

  it('shows disabled state when the feature flag is off', () => {
    cachedPublicSettings.value = { chat_completion_enabled: false }

    const wrapper = mount(ChatCompletionView)

    expect(wrapper.text()).toContain('chatCompletion.disabled')
    expect(wrapper.find('[data-testid="chat-workbench"]').exists()).toBe(false)
  })

  it('fills the composer when a prompt suggestion is selected', async () => {
    const wrapper = mount(ChatCompletionView)
    await flushPromises()

    await wrapper.get('[data-testid="chat-suggestion-0"]').trigger('click')

    const textarea = wrapper.get<HTMLTextAreaElement>('[data-testid="chat-message-input"]')
    expect(textarea.element.value).toBe('chatCompletion.suggestion1')
  })

  it('enables sending only after key, model, and draft are ready', async () => {
    const wrapper = mount(ChatCompletionView)
    await flushPromises()

    const sendButton = wrapper.get<HTMLButtonElement>('[data-testid="chat-send-button"]')
    expect(sendButton.element.disabled).toBe(true)

    await wrapper.get('[data-testid="chat-message-input"]').setValue('hello')

    expect(sendButton.element.disabled).toBe(false)
  })

  it('uses a spacious readable composer input', async () => {
    const wrapper = mount(ChatCompletionView)
    await flushPromises()

    const textarea = wrapper.get('[data-testid="chat-message-input"]')

    expect(wrapper.get('[data-testid="chat-composer-shell"]').classes()).toContain('rounded-[28px]')
    expect(textarea.classes()).toContain('min-h-[84px]')
    expect(textarea.classes()).toContain('text-base')
    expect(textarea.classes()).toContain('text-gray-950')
    expect(textarea.classes()).toContain('dark:text-gray-100')
    expect(textarea.classes()).toContain('placeholder:text-gray-500')
    expect(textarea.classes()).toContain('dark:placeholder:text-gray-400')
  })

  it('uses compact icon controls inside the floating composer', async () => {
    const wrapper = mount(ChatCompletionView)
    await flushPromises()
    await wrapper.get('[data-testid="chat-message-input"]').setValue('hello')

    const sendButton = wrapper.get('[data-testid="chat-send-button"]')
    const regenerateButton = wrapper.get('[data-testid="chat-regenerate-button"]')

    expect(sendButton.attributes('aria-label')).toBe('chatCompletion.send')
    expect(regenerateButton.attributes('aria-label')).toBe('chatCompletion.regenerate')
    expect(sendButton.text()).toBe('')
    expect(regenerateButton.text()).toBe('')
  })

  it('loads server-backed sessions into the history sidebar', async () => {
    listChatSessions.mockResolvedValueOnce([
      {
        id: 77,
        api_key_id: 1,
        title: 'Saved chat',
        model: 'gpt-5.4',
        status: 'active',
        expires_at: '2026-06-12T00:00:00Z',
        created_at: '2026-05-13T00:00:00Z',
        updated_at: '2026-05-13T00:00:00Z',
      },
    ])
    const wrapper = mount(ChatCompletionView)
    await flushPromises()

    expect(wrapper.find('[data-testid="chat-session-sidebar"]').text()).toContain('Saved chat')
    expect(wrapper.find('[data-testid="chat-session-sidebar"]').text()).not.toContain('05/13')
  })

  it('selects a session and loads its persisted messages', async () => {
    listChatSessions.mockResolvedValueOnce([
      {
        id: 77,
        api_key_id: 1,
        title: 'Saved chat',
        model: 'gpt-5.4',
        status: 'active',
        expires_at: '2026-06-12T00:00:00Z',
        created_at: '2026-05-13T00:00:00Z',
        updated_at: '2026-05-13T00:00:00Z',
      },
    ])
    getChatSessionMessages.mockResolvedValueOnce([
      {
        id: 301,
        session_id: 77,
        role: 'assistant',
        content: '**saved**',
        status: 'completed',
        created_at: '2026-05-13T00:00:00Z',
        updated_at: '2026-05-13T00:00:00Z',
      },
    ])
    const wrapper = mount(ChatCompletionView)
    await flushPromises()

    await wrapper.get('[data-testid="chat-session-item"]').trigger('click')
    await flushPromises()

    expect(getChatSessionMessages).toHaveBeenCalledWith(77)
    expect(wrapper.text()).toContain('**saved**')
  })

  it('sends the selected model, persists messages, and clears the draft', async () => {
    const wrapper = mount(ChatCompletionView)
    await flushPromises()
    await wrapper.get('[data-testid="chat-message-input"]').setValue('hello')
    await wrapper.get('[data-testid="chat-composer"]').trigger('submit')
    await flushPromises()

    expect(createChatSession).toHaveBeenCalledWith({
      api_key_id: 1,
      title: 'hello',
      model: 'gpt-5.4',
    })
    expect(createChatMessage).toHaveBeenCalledWith(100, expect.objectContaining({
      role: 'user',
      content: 'hello',
      status: 'completed',
    }))
    expect(streamChatCompletion).toHaveBeenCalledWith(expect.objectContaining({
      apiKey: 'sk-active',
      model: 'gpt-5.4',
      platform: 'openai',
      messages: [{ role: 'user', content: 'hello' }],
      promptCacheKey: 'chat-session-100',
    }))
    expect(updateChatMessage).toHaveBeenCalledWith(100, 202, expect.objectContaining({
      status: 'completed',
    }))
    expect(wrapper.get<HTMLTextAreaElement>('[data-testid="chat-message-input"]').element.value).toBe('')
  })

  it('does not render role labels in chat bubbles', async () => {
    streamChatCompletion.mockImplementationOnce(({ onDelta }) => {
      onDelta('Saved answer')
      return Promise.resolve()
    })
    const wrapper = mount(ChatCompletionView)
    await flushPromises()
    await wrapper.get('[data-testid="chat-message-input"]').setValue('hello')
    await wrapper.get('[data-testid="chat-composer"]').trigger('submit')
    await flushPromises()

    expect(wrapper.text()).not.toContain('chatCompletion.user')
    expect(wrapper.text()).not.toContain('chatCompletion.assistant')
  })

  it('renders per-message copy buttons and copies message content', async () => {
    getChatSessionMessages.mockResolvedValueOnce([
      {
        id: 300,
        session_id: 77,
        role: 'user',
        content: 'Saved question',
        status: 'completed',
        created_at: '2026-05-13T00:00:00Z',
        updated_at: '2026-05-13T00:00:00Z',
      },
      {
        id: 301,
        session_id: 77,
        role: 'assistant',
        content: 'Saved answer',
        status: 'completed',
        created_at: '2026-05-13T00:00:00Z',
        updated_at: '2026-05-13T00:00:00Z',
      },
    ])
    listChatSessions.mockResolvedValueOnce([
      {
        id: 77,
        api_key_id: 1,
        title: 'Saved chat',
        model: 'gpt-5.4',
        status: 'active',
        expires_at: '2026-06-12T00:00:00Z',
        created_at: '2026-05-13T00:00:00Z',
        updated_at: '2026-05-13T00:00:00Z',
      },
    ])
    const wrapper = mount(ChatCompletionView)
    await flushPromises()
    await wrapper.get('[data-testid="chat-session-item"]').trigger('click')
    await flushPromises()

    const buttons = wrapper.findAll('[data-testid="chat-copy-message-button"]')
    expect(buttons).toHaveLength(2)
    expect(buttons[0].classes()).toContain('h-8')
    expect(buttons[0].classes()).toContain('w-8')

    await buttons[0].trigger('click')
    await buttons[1].trigger('click')

    expect(copyToClipboard).toHaveBeenNthCalledWith(1, 'Saved question', 'chatCompletion.copied')
    expect(copyToClipboard).toHaveBeenNthCalledWith(2, 'Saved answer', 'chatCompletion.copied')
  })

  it('jumps to the bottom without smooth animation when loading a history conversation', async () => {
    const scrollTo = vi.fn()
    Element.prototype.scrollTo = scrollTo
    getChatSessionMessages.mockResolvedValueOnce([
      {
        id: 301,
        session_id: 77,
        role: 'assistant',
        content: 'Saved answer',
        status: 'completed',
        created_at: '2026-05-13T00:00:00Z',
        updated_at: '2026-05-13T00:00:00Z',
      },
    ])
    listChatSessions.mockResolvedValueOnce([
      {
        id: 77,
        api_key_id: 1,
        title: 'Saved chat',
        model: 'gpt-5.4',
        status: 'active',
        expires_at: '2026-06-12T00:00:00Z',
        created_at: '2026-05-13T00:00:00Z',
        updated_at: '2026-05-13T00:00:00Z',
      },
    ])
    const wrapper = mount(ChatCompletionView)
    await flushPromises()
    await wrapper.get('[data-testid="chat-session-item"]').trigger('click')
    await flushPromises()

    expect(scrollTo).toHaveBeenCalledWith(expect.objectContaining({
      behavior: 'auto',
    }))
  })

  it('shows an active generating indicator before the first streamed token without cost noise', async () => {
    let resolveStream: (() => void) | undefined
    streamChatCompletion.mockImplementationOnce(({ onDelta }) => new Promise<void>((resolve) => {
      resolveStream = () => {
        onDelta('hello')
        resolve()
      }
    }))
    const wrapper = mount(ChatCompletionView)
    await flushPromises()
    await wrapper.get('[data-testid="chat-message-input"]').setValue('hello')
    await wrapper.get('[data-testid="chat-composer"]').trigger('submit')
    await flushPromises()

    expect(wrapper.find('[data-testid="chat-generating-indicator"]').exists()).toBe(true)
    expect(wrapper.text()).toContain('chatCompletion.generatingReply')
    expect(wrapper.text()).not.toContain('chatCompletion.currentCostPending')

    resolveStream?.()
    await flushPromises()
  })

  it('renders streamed deltas before the request finishes', async () => {
    let resolveStream: (() => void) | undefined
    let emitDelta: ((delta: string) => void) | undefined
    streamChatCompletion.mockImplementationOnce(({ onDelta }) => new Promise<void>((resolve) => {
      emitDelta = onDelta
      resolveStream = resolve
    }))
    const wrapper = mount(ChatCompletionView)
    await flushPromises()
    await wrapper.get('[data-testid="chat-message-input"]').setValue('hello')
    await wrapper.get('[data-testid="chat-composer"]').trigger('submit')
    await flushPromises()

    emitDelta?.('he')
    await flushPromises()
    expect(wrapper.text()).toContain('he')

    emitDelta?.('llo')
    await flushPromises()
    expect(wrapper.text()).toContain('hello')

    resolveStream?.()
    await flushPromises()
  })

  it('does not keep showing generating after an empty completed response', async () => {
    const wrapper = mount(ChatCompletionView)
    await flushPromises()
    await wrapper.get('[data-testid="chat-message-input"]').setValue('hello')
    await wrapper.get('[data-testid="chat-composer"]').trigger('submit')
    await flushPromises()

    expect(wrapper.find('[data-testid="chat-generating-indicator"]').exists()).toBe(false)
    expect(wrapper.text()).toContain('chatCompletion.emptyReply')
    expect(wrapper.text()).not.toContain('chatCompletion.generatingReply')
  })

  it('does not show completion status, duration, or cost metadata under assistant replies', async () => {
    updateChatMessage.mockResolvedValueOnce({
      id: 202,
      session_id: 100,
      role: 'assistant',
      content: 'done',
      status: 'completed',
      model: 'gpt-5.4',
      duration_ms: 1240,
      actual_cost: 0.000123,
      created_at: '2026-05-13T00:00:00Z',
      updated_at: '2026-05-13T00:00:00Z',
    })
    streamChatCompletion.mockImplementationOnce(({ onDelta }) => {
      onDelta('done')
      return Promise.resolve()
    })

    const wrapper = mount(ChatCompletionView)
    await flushPromises()
    await wrapper.get('[data-testid="chat-message-input"]').setValue('hello')
    await wrapper.get('[data-testid="chat-composer"]').trigger('submit')
    await flushPromises()

    expect(wrapper.text()).toContain('done')
    expect(wrapper.text()).not.toContain('chatCompletion.completed')
    expect(wrapper.text()).not.toContain('chatCompletion.duration')
    expect(wrapper.text()).not.toContain('chatCompletion.currentCost')
    expect(wrapper.text()).not.toContain('chatCompletion.currentCostPending')
  })

  it('scrolls the message list to the bottom as messages stream in', async () => {
    const scrollTo = vi.fn()
    Element.prototype.scrollTo = scrollTo
    let resolveStream: (() => void) | undefined
    let emitDelta: ((delta: string) => void) | undefined
    streamChatCompletion.mockImplementationOnce(({ onDelta }) => new Promise<void>((resolve) => {
      emitDelta = onDelta
      resolveStream = resolve
    }))
    const wrapper = mount(ChatCompletionView)
    await flushPromises()
    await wrapper.get('[data-testid="chat-message-input"]').setValue('hello')
    await wrapper.get('[data-testid="chat-composer"]').trigger('submit')
    await flushPromises()

    emitDelta?.('streaming')
    await flushPromises()

    expect(scrollTo).toHaveBeenCalled()
    resolveStream?.()
    await flushPromises()
  })

  it('does not send while IME composition is active', async () => {
    const wrapper = mount(ChatCompletionView)
    await flushPromises()
    const textarea = wrapper.get('[data-testid="chat-message-input"]')
    await textarea.setValue('ni')

    await textarea.trigger('keydown', {
      key: 'Enter',
      isComposing: true,
    })
    await flushPromises()

    expect(streamChatCompletion).not.toHaveBeenCalled()
    expect(createChatMessage).not.toHaveBeenCalled()
  })

  it('does not send Enter key events emitted as IME process key 229', async () => {
    const wrapper = mount(ChatCompletionView)
    await flushPromises()
    const textarea = wrapper.get('[data-testid="chat-message-input"]')
    await textarea.setValue('ni')

    await textarea.trigger('keydown', {
      key: 'Enter',
      keyCode: 229,
    })
    await flushPromises()

    expect(streamChatCompletion).not.toHaveBeenCalled()
    expect(createChatMessage).not.toHaveBeenCalled()
  })

  it('shows regenerate control for existing conversations', async () => {
    const wrapper = mount(ChatCompletionView)
    await flushPromises()
    await wrapper.get('[data-testid="chat-message-input"]').setValue('hello')
    await wrapper.get('[data-testid="chat-composer"]').trigger('submit')
    await flushPromises()

    expect(wrapper.get<HTMLButtonElement>('[data-testid="chat-regenerate-button"]').element.disabled).toBe(false)
  })
})
