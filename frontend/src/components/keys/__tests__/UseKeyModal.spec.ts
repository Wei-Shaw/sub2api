import { beforeEach, describe, expect, it, vi } from 'vitest'
import { mount } from '@vue/test-utils'
import { nextTick } from 'vue'

vi.mock('vue-i18n', () => ({
  useI18n: () => ({
    t: (key: string) => key
  })
}))

vi.mock('@/composables/useClipboard', () => ({
  useClipboard: () => ({
    copyToClipboard: vi.fn().mockResolvedValue(true)
  })
}))

import UseKeyModal from '../UseKeyModal.vue'

describe('UseKeyModal', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  it('renders GPT-5.4 mini entry in OpenCode config', async () => {
    const wrapper = mount(UseKeyModal, {
      props: {
        show: true,
        apiKey: 'sk-test',
        baseUrl: 'https://example.com/v1',
        platform: 'openai'
      },
      global: {
        stubs: {
          BaseDialog: {
            template: '<div><slot /><slot name="footer" /></div>'
          },
          Icon: {
            template: '<span />'
          }
        }
      }
    })

    const opencodeTab = wrapper.findAll('button').find((button) =>
      button.text().includes('keys.useKeyModal.cliTabs.opencode')
    )

    expect(opencodeTab).toBeDefined()
    await opencodeTab!.trigger('click')
    await nextTick()

    const codeBlock = wrapper.find('pre code')
    expect(codeBlock.exists()).toBe(true)
    expect(codeBlock.text()).toContain('"name": "GPT-5.4 Mini"')
    expect(codeBlock.text()).not.toContain('"name": "GPT-5.4 Nano"')
  })

  it('applies matching rewrite rules to content only', async () => {
    const wrapper = mount(UseKeyModal, {
      props: {
        show: true,
        apiKey: 'sk-test',
        baseUrl: 'https://example.com/v1',
        platform: 'openai',
        aiToolRewriteRules: [
          {
            enabled: true,
            platform: 'openai',
            client: 'codex',
            find: 'model = "gpt-5.4"',
            replace: 'model = "gpt-5.5"'
          },
          {
            enabled: true,
            platform: '',
            client: '',
            find: '~/.codex/config.toml',
            replace: '~/.other/config.toml'
          }
        ]
      },
      global: {
        stubs: {
          BaseDialog: {
            template: '<div><slot /><slot name="footer" /></div>'
          },
          Icon: {
            template: '<span />'
          }
        }
      }
    })

    const codeBlocks = wrapper.findAll('pre code')
    expect(codeBlocks[0]?.text()).toContain('model = "gpt-5.5"')
    expect(codeBlocks[0]?.text()).not.toContain('model = "gpt-5.4"')
    expect(wrapper.text()).toContain('~/.codex/config.toml')
    expect(wrapper.text()).not.toContain('~/.other/config.toml')
  })

  it('skips disabled and non-matching rewrite rules', () => {
    const wrapper = mount(UseKeyModal, {
      props: {
        show: true,
        apiKey: 'sk-test',
        baseUrl: 'https://example.com/v1',
        platform: 'openai',
        aiToolRewriteRules: [
          {
            enabled: false,
            platform: 'openai',
            client: 'codex',
            find: 'gpt-5.4',
            replace: 'disabled-model'
          },
          {
            enabled: true,
            platform: 'gemini',
            client: 'gemini',
            find: 'gpt-5.4',
            replace: 'wrong-platform'
          }
        ]
      },
      global: {
        stubs: {
          BaseDialog: {
            template: '<div><slot /><slot name="footer" /></div>'
          },
          Icon: {
            template: '<span />'
          }
        }
      }
    })

    const codeBlock = wrapper.find('pre code')
    expect(codeBlock.text()).toContain('gpt-5.4')
    expect(codeBlock.text()).not.toContain('disabled-model')
    expect(codeBlock.text()).not.toContain('wrong-platform')
  })
})
