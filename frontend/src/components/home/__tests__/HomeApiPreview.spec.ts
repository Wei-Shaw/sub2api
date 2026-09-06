import { flushPromises, mount } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'

import HomeApiPreview from '../HomeApiPreview.vue'

vi.mock('vue-i18n', () => ({
  useI18n: () => ({ t: (key: string) => key })
}))

describe('HomeApiPreview', () => {
  beforeEach(() => {
    document.body.innerHTML = ''
  })

  it('switches all protocol examples and composes the configured API base', async () => {
    const wrapper = mount(HomeApiPreview, {
      props: { apiBaseUrl: 'https://gateway.example.com/' },
      attachTo: document.body
    })
    const tabs = wrapper.findAll<HTMLButtonElement>('[role="tab"]')

    expect(tabs).toHaveLength(4)
    expect(tabs[0].attributes('aria-selected')).toBe('true')
    expect(wrapper.find('.api-preview__endpoint').attributes('title')).toBe(
      'https://gateway.example.com/v1/chat/completions'
    )

    await tabs[1].trigger('click')
    await flushPromises()
    await vi.waitFor(() => expect(wrapper.text()).toContain('OpenAI Responses'))
    expect(tabs[1].attributes('aria-selected')).toBe('true')
    expect(wrapper.text()).toContain('/v1/responses')

    await tabs[2].trigger('click')
    await flushPromises()
    await vi.waitFor(() => expect(wrapper.text()).toContain('claude-sonnet-4'))
    expect(wrapper.text()).toContain('/v1/messages')

    await tabs[3].trigger('click')
    await flushPromises()
    await vi.waitFor(() => expect(wrapper.text()).toContain('Google AI native'))
    expect(wrapper.text()).toContain('/v1beta/models/gemini-2.5-flash:generateContent')
    expect(wrapper.find('.api-preview__endpoint').attributes('title')).toBe(
      'https://gateway.example.com/v1beta/models/gemini-2.5-flash:generateContent'
    )
  })

  it('supports roving focus with arrow, Home, and End keys', async () => {
    const wrapper = mount(HomeApiPreview, {
      props: { apiBaseUrl: 'https://gateway.example.com' },
      attachTo: document.body
    })
    const tabs = wrapper.findAll<HTMLButtonElement>('[role="tab"]')

    await tabs[0].trigger('keydown', { key: 'ArrowRight' })
    expect(tabs[1].attributes('aria-selected')).toBe('true')
    expect(document.activeElement).toBe(tabs[1].element)

    await tabs[1].trigger('keydown', { key: 'End' })
    expect(tabs[3].attributes('aria-selected')).toBe('true')
    expect(document.activeElement).toBe(tabs[3].element)

    await tabs[3].trigger('keydown', { key: 'Home' })
    expect(tabs[0].attributes('aria-selected')).toBe('true')
    expect(document.activeElement).toBe(tabs[0].element)
  })
})
