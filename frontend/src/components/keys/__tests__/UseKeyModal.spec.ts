import { describe, expect, it, vi } from 'vitest'
import { mount } from '@vue/test-utils'
import { nextTick } from 'vue'
import { readFileSync } from 'node:fs'
import { resolve } from 'node:path'

const { openaiModelsMock } = vi.hoisted(() => ({
  openaiModelsMock: {
  'gpt-5.4': {
    id: 'gpt-5.4',
    name: 'GPT-5.4',
    attachment: true,
    reasoning: true,
    tool_call: true,
    modalities: {
      input: ['text', 'image', 'pdf'],
      output: ['text']
    },
    limit: {
      context: 1050000,
      input: 922000,
      output: 128000
    },
    cost: {
      input: 2.5,
      output: 15,
      cache_read: 0.25,
      context_over_200k: {
        input: 5,
        output: 22.5,
        cache_read: 0.5
      }
    },
    release_date: '2026-03-05'
  },
  'gpt-5.4-mini': {
    id: 'gpt-5.4-mini',
    name: 'GPT-5.4 Mini',
    attachment: true,
    reasoning: true,
    tool_call: true,
    modalities: {
      input: ['text', 'image'],
      output: ['text']
    },
    limit: {
      context: 400000,
      input: 272000,
      output: 128000
    },
    cost: {
      input: 0.75,
      output: 4.5,
      cache_read: 0.075
    },
    release_date: '2026-03-17'
  },
  'gpt-5.4-nano': {
    id: 'gpt-5.4-nano',
    name: 'GPT-5.4 Nano',
    attachment: true,
    reasoning: true,
    tool_call: true,
    modalities: {
      input: ['text', 'image'],
      output: ['text']
    },
    limit: {
      context: 400000,
      input: 272000,
      output: 128000
    },
    cost: {
      input: 0.2,
      output: 1.25,
      cache_read: 0.02
    },
    release_date: '2026-03-17'
  },
  'gpt-5.2': {
    id: 'gpt-5.2',
    name: 'GPT-5.2',
    attachment: true,
    reasoning: true,
    tool_call: true,
    modalities: {
      input: ['text', 'image'],
      output: ['text']
    },
    limit: {
      context: 400000,
      input: 272000,
      output: 128000
    },
    cost: {
      input: 1.75,
      output: 14,
      cache_read: 0.175
    },
    release_date: '2025-12-11'
  }
  }
}))

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

vi.mock('@/api/keys', () => ({
  keysAPI: {
    getOpenCodeOpenAIModels: vi.fn().mockResolvedValue({
      models: openaiModelsMock
    })
  }
}))

import UseKeyModal from '../UseKeyModal.vue'

describe('UseKeyModal', () => {
  it('renders updated GPT-5.4 mini/nano names in OpenCode config', async () => {
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
    await Promise.resolve()
    await nextTick()

    const codeBlock = wrapper.find('pre code')
    expect(codeBlock.exists()).toBe(true)
    expect(codeBlock.text()).toContain('"name": "GPT-5.4 Mini"')
    expect(codeBlock.text()).toContain('"name": "GPT-5.4 Nano"')
  })

  it('renders sub2api-openai provider config with Sys models in OpenCode example', async () => {
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
    await Promise.resolve()
    await nextTick()

    const codeBlock = wrapper.find('pre code')
    expect(codeBlock.exists()).toBe(true)
    expect(codeBlock.text()).toContain('"sub2api-openai"')
    expect(codeBlock.text()).toContain('"baseURL": "https://example.com/v1"')
    expect(codeBlock.text()).toContain('"gpt-5.4-Sys"')
    expect(codeBlock.text()).toContain('"name": "GPT-5.4 (Sys)"')
    expect(codeBlock.text()).not.toContain('"provider": {\n    "openai"')

    const parsed = JSON.parse(codeBlock.text())
    const gpt54Variants = parsed.provider['sub2api-openai'].models['gpt-5.4'].variants
    const gpt54SysVariants = parsed.provider['sub2api-openai'].models['gpt-5.4-Sys'].variants
    const gpt52Variants = parsed.provider['sub2api-openai'].models['gpt-5.2'].variants
    const gpt54 = parsed.provider['sub2api-openai'].models['gpt-5.4']
    const gpt54Sys = parsed.provider['sub2api-openai'].models['gpt-5.4-Sys']

    expect(gpt54.attachment).toBe(true)
    expect(gpt54.modalities.input).toEqual(expect.arrayContaining(['text', 'image', 'pdf']))
    expect(gpt54.modalities.output).toEqual(['text'])
    expect(gpt54Sys.attachment).toBe(true)
    expect(gpt54Sys.modalities.input).toEqual(expect.arrayContaining(['text', 'image', 'pdf']))
    expect(gpt54Sys.modalities.output).toEqual(['text'])
    expect(gpt54Variants['low-fast'].serviceTier).toBe('priority')
    expect(gpt54Variants['medium-fast'].serviceTier).toBe('priority')
    expect(gpt54Variants['high-fast'].serviceTier).toBe('priority')
    expect(gpt54Variants['xhigh-fast'].serviceTier).toBe('priority')
    expect(gpt54Variants['xhigh-fast'].reasoningEffort).toBe('xhigh')
    expect(gpt54SysVariants['xhigh-fast'].serviceTier).toBe('priority')
    expect(gpt54SysVariants['xhigh-fast'].reasoningEffort).toBe('xhigh')
    expect(gpt54Variants['fast-low']).toBeUndefined()
    expect(gpt54SysVariants['fast-low']).toBeUndefined()
    expect(gpt52Variants['low-fast']).toBeUndefined()
  })

  it('describes OpenCode config as custom provider based', async () => {
    const zhLocale = readFileSync(resolve(__dirname, '../../../i18n/locales/zh.ts'), 'utf8')
    const enLocale = readFileSync(resolve(__dirname, '../../../i18n/locales/en.ts'), 'utf8')

    expect(zhLocale).toContain('provider_id')
    expect(zhLocale).toContain('sub2api-openai')
    expect(enLocale).toContain('custom provider_id')
    expect(enLocale).toContain('sub2api-openai')
  })
})
