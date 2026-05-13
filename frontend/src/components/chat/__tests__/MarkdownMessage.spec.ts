import { beforeEach, describe, expect, it, vi } from 'vitest'
import { mount } from '@vue/test-utils'
import MarkdownMessage from '../MarkdownMessage.vue'

const copyToClipboard = vi.hoisted(() => vi.fn())

vi.mock('@/composables/useClipboard', () => ({
  useClipboard: () => ({ copyToClipboard }),
}))

vi.mock('vue-i18n', () => ({
  useI18n: () => ({
    t: (key: string) => key,
  }),
}))

describe('MarkdownMessage', () => {
  beforeEach(() => {
    copyToClipboard.mockReset()
  })

  it('renders markdown and sanitizes unsafe html', () => {
    const wrapper = mount(MarkdownMessage, {
      props: {
        content: 'Hello **world**\n\n- one\n\n<script>alert(1)</script>',
      },
    })

    expect(wrapper.html()).toContain('<strong>world</strong>')
    expect(wrapper.html()).toContain('<li>one</li>')
    expect(wrapper.html()).not.toContain('<script>')
  })

  it('renders each fenced code block with an inline copy button', async () => {
    const wrapper = mount(MarkdownMessage, {
      props: {
        content: '```ts\nconst answer = 42\n```\n\n```bash\nnpm test\n```',
      },
    })

    expect(wrapper.html()).toContain('const answer = 42')
    expect(wrapper.find('[data-testid="copy-code-button-list"]').exists()).toBe(false)
    expect(wrapper.findAll('[data-testid="chat-code-block"]').length).toBe(2)
    expect(wrapper.findAll('[data-testid="copy-code-button"]').length).toBe(2)
    await wrapper.findAll('[data-testid="copy-code-button"]')[0].trigger('click')

    expect(copyToClipboard).toHaveBeenCalledWith('const answer = 42')
  })

  it('keeps long fenced code constrained to the message width', () => {
    const wrapper = mount(MarkdownMessage, {
      props: {
        content: `\`\`\`java\n${'System.out.println("very long code line");'.repeat(20)}\n\`\`\``,
      },
    })

    expect(wrapper.classes()).toEqual(expect.arrayContaining(['min-w-0', 'max-w-full', 'overflow-hidden']))
    expect(wrapper.find('[data-testid="chat-code-block"]').classes()).toEqual(
      expect.arrayContaining(['min-w-0', 'max-w-full', 'overflow-hidden']),
    )
    expect(wrapper.find('pre').classes()).toEqual(expect.arrayContaining(['max-w-full', 'overflow-x-auto']))
    expect(wrapper.find('pre code').classes()).toEqual(expect.arrayContaining(['block', 'w-max', 'min-w-full']))
  })
})
