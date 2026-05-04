import { describe, expect, it, vi } from 'vitest'
import { mount } from '@vue/test-utils'
import { nextTick } from 'vue'
import { readFileSync } from 'node:fs'
import { resolve } from 'node:path'

const { openaiModelsMock } = vi.hoisted(() => ({
  openaiModelsMock: {
  'gpt-5.5': {
    id: 'gpt-5.5',
    name: 'GPT-5.5',
    attachment: true,
    reasoning: true,
    tool_call: true,
    structured_output: true,
    temperature: false,
    modalities: {
      input: ['text', 'image', 'pdf'],
      output: ['text']
    },
    limit: {
      context: 400000,
      input: 272000,
      output: 128000
    },
    cost: {
      input: 3,
      output: 18,
      cache_read: 0.3
    },
    release_date: '2026-04-23'
  },
  'gpt-5.5-fast': {
    id: 'gpt-5.5-fast',
    name: 'GPT-5.5 Fast',
    attachment: true,
    reasoning: true,
    tool_call: true,
    structured_output: true,
    temperature: false,
    modalities: {
      input: ['text', 'image', 'pdf'],
      output: ['text']
    },
    limit: {
      context: 400000,
      input: 272000,
      output: 128000
    },
    cost: {
      input: 6,
      output: 36,
      cache_read: 0.6
    },
    release_date: '2026-04-23',
    options: {
      serviceTier: 'priority'
    },
    headers: {
      'x-test-header': 'gpt-5.5-fast-mode'
    }
  },
  'gpt-5.4': {
    id: 'gpt-5.4',
    name: 'GPT-5.4',
    attachment: true,
    reasoning: true,
    tool_call: true,
    structured_output: true,
    temperature: false,
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
    release_date: '2026-03-05',
    experimental: {
      modes: {
        fast: {
          provider: {
            body: {
              service_tier: 'wrong-from-raw'
            },
            headers: {
              'x-test-header': 'wrong-raw-header'
            }
          }
        }
      }
    },
    provider: {
      body: {
        service_tier: 'default'
      },
      headers: {
        'x-base-header': 'base-mode'
      }
    },
    tools: [{ type: 'web_search' }]
  },
  'gpt-5.4-fast': {
    id: 'gpt-5.4-fast',
    name: 'GPT-5.4 Fast',
    attachment: true,
    reasoning: true,
    tool_call: true,
    structured_output: true,
    temperature: false,
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
      input: 5,
      output: 30,
      cache_read: 0.5,
      context_over_200k: {
        input: 10,
        output: 45,
        cache_read: 1
      }
    },
    release_date: '2026-03-05',
    options: {
      serviceTier: 'priority'
    },
    headers: {
      'x-test-header': 'fast-mode'
    },
    experimental: {
      leaked: true
    },
    provider: {
      body: {
        service_tier: 'priority'
      },
      headers: {
        'x-test-header': 'fast-mode'
      }
    },
    tools: [{ type: 'web_search' }]
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
  it('renders GPT-5.4 small-model entries in OpenCode config', async () => {
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
    expect(codeBlock.text()).toContain('"gpt-5.5"')
    expect(codeBlock.text()).toContain('"gpt-5.5-fast"')
    expect(codeBlock.text()).toContain('"gpt-5.5-Sys"')
    expect(codeBlock.text()).toContain('"gpt-5.5-fast-Sys"')
    expect(codeBlock.text()).toContain('"gpt-5.4-fast"')
    expect(codeBlock.text()).toContain('"gpt-5.4-Sys"')
    expect(codeBlock.text()).toContain('"gpt-5.4-fast-Sys"')
    expect(codeBlock.text()).toContain('"name": "GPT-5.4 (Sys)"')
    expect(codeBlock.text()).not.toContain('"provider": {\n    "openai"')

    const parsed = JSON.parse(codeBlock.text())
    const sub2apiProvider = parsed.provider['sub2api-openai']
    const models = sub2apiProvider.models
    const modelsJson = JSON.stringify(models)
    const gpt54 = models['gpt-5.4']
    const gpt54Fast = models['gpt-5.4-fast']
    const gpt54Sys = models['gpt-5.4-Sys']
    const gpt54FastSys = models['gpt-5.4-fast-Sys']
    const gpt54Mini = models['gpt-5.4-mini']
    const gpt55 = models['gpt-5.5']
    const gpt55Fast = models['gpt-5.5-fast']
    const gpt55Sys = models['gpt-5.5-Sys']
    const gpt55FastSys = models['gpt-5.5-fast-Sys']
    const gpt54Variants = gpt54.variants ?? {}
    const gpt54SysVariants = gpt54Sys.variants ?? {}
    const gpt54MiniVariants = gpt54Mini.variants ?? {}
    const gpt55Variants = gpt55.variants ?? {}

    expect(sub2apiProvider).toBeDefined()
    expect(sub2apiProvider.npm).toBe('@ai-sdk/openai')
    expect(parsed.provider.openai).toBeUndefined()
    expect(gpt54).toBeDefined()
    expect(gpt54Fast).toBeDefined()
    expect(gpt54Sys).toBeDefined()
    expect(gpt54FastSys).toBeDefined()
    expect(gpt55).toBeDefined()
    expect(gpt55Fast).toBeDefined()
    expect(gpt55Sys).toBeDefined()
    expect(gpt55FastSys).toBeDefined()
    expect(gpt54.attachment).toBe(true)
    expect(gpt54.modalities.input).toEqual(expect.arrayContaining(['text', 'image', 'pdf']))
    expect(gpt54.modalities.output).toEqual(['text'])
    expect(gpt54.id).toBe('gpt-5.4')
    expect(gpt54Fast.id).toBe('gpt-5.4')
    expect(gpt54Sys.id).toBe('gpt-5.4-Sys')
    expect(gpt54FastSys.id).toBe('gpt-5.4-Sys')
    expect(gpt55.id).toBe('gpt-5.5')
    expect(gpt55Fast.id).toBe('gpt-5.5')
    expect(gpt55Sys.id).toBe('gpt-5.5-Sys')
    expect(gpt55FastSys.id).toBe('gpt-5.5-Sys')
    expect(gpt55.limit).toMatchObject({ context: 400000, input: 272000, output: 128000 })
    expect(gpt55Fast.limit).toMatchObject({ context: 400000, input: 272000, output: 128000 })
    expect(gpt55Sys.limit).toMatchObject({ context: 400000, input: 272000, output: 128000 })
    expect(gpt55FastSys.limit).toMatchObject({ context: 400000, input: 272000, output: 128000 })
    expect(gpt54.cost.context_over_200k).toBeUndefined()
    expect(gpt54Fast.cost.context_over_200k).toBeUndefined()
    expect(gpt54Mini.cost.context_over_200k).toBeUndefined()
    expect(gpt54Sys.attachment).toBe(true)
    expect(gpt54Sys.modalities.input).toEqual(expect.arrayContaining(['text', 'image', 'pdf']))
    expect(gpt54Sys.modalities.output).toEqual(['text'])
    expect(gpt54.options.builtin_tools).toBeUndefined()
    expect(gpt54.options.metadata.builtin_tools).toEqual({ web_search: true })
    expect(gpt54Fast.options.serviceTier).toBe('priority')
    expect(gpt54Fast.options.builtin_tools).toBeUndefined()
    expect(gpt54Fast.options.metadata.builtin_tools).toEqual({ web_search: true })
    expect(gpt54Fast.headers['x-test-header']).toBe('fast-mode')
    expect(gpt54Sys.options.builtin_tools).toBeUndefined()
    expect(gpt54Sys.options.metadata.builtin_tools).toEqual({ web_search: true })
    expect(gpt54FastSys.options.serviceTier).toBe('priority')
    expect(gpt54FastSys.options.builtin_tools).toBeUndefined()
    expect(gpt54FastSys.options.metadata.builtin_tools).toEqual({ web_search: true })
    expect(gpt54FastSys.headers['x-test-header']).toBe('fast-mode')
    expect(gpt54Mini.options.builtin_tools).toBeUndefined()
    expect(gpt54Mini.options.metadata.builtin_tools).toEqual({ web_search: true })
    const gpt55ImageBuiltinTools = {
      web_search: true,
      image_generation: {
        enabled: true,
        model: 'gpt-image-2',
        output_format: 'png'
      }
    }

    for (const model of [gpt55, gpt55Fast, gpt55Sys, gpt55FastSys]) {
      expect(model.options.metadata.builtin_tools).toEqual({ web_search: true })
      expect(model.variants.image).toBeDefined()
      expect(model.variants.image.metadata.builtin_tools).toEqual(gpt55ImageBuiltinTools)
    }

    expect(gpt55Fast.options.serviceTier).toBe('priority')
    expect(gpt55Fast.headers['x-test-header']).toBe('gpt-5.5-fast-mode')
    expect(gpt55FastSys.options.serviceTier).toBe('priority')
    expect(gpt55FastSys.headers['x-test-header']).toBe('gpt-5.5-fast-mode')
    expect(gpt54.tools).toBeUndefined()
    expect(gpt54Fast.tools).toBeUndefined()
    expect(gpt54Sys.tools).toBeUndefined()
    expect(gpt54FastSys.tools).toBeUndefined()
    expect(gpt54Mini.tools).toBeUndefined()
    expect(gpt55.tools).toBeUndefined()
    expect(gpt55Fast.tools).toBeUndefined()
    expect(gpt55Sys.tools).toBeUndefined()
    expect(gpt55FastSys.tools).toBeUndefined()
    expect(gpt54Variants['low-fast']).toBeUndefined()
    expect(gpt54Variants['medium-fast']).toBeUndefined()
    expect(gpt54Variants['high-fast']).toBeUndefined()
    expect(gpt54Variants['xhigh-fast']).toBeUndefined()
    expect(Object.keys(gpt54Variants).some((variant) => variant.endsWith('-fast'))).toBe(false)
    expect(Object.keys(gpt54SysVariants).some((variant) => variant.endsWith('-fast'))).toBe(false)
    expect(Object.keys(gpt54MiniVariants).some((variant) => variant.endsWith('-fast'))).toBe(false)
    expect(gpt55Variants.none).toBeDefined()
    expect(gpt55Variants.low).toBeDefined()
    expect(gpt55Variants.medium).toBeDefined()
    expect(gpt55Variants.high).toBeDefined()
    expect(gpt55Variants.xhigh).toBeDefined()
    expect(gpt55Variants.image).toBeDefined()
    expect(gpt55Sys.variants).not.toBe(gpt55.variants)
    expect(gpt55FastSys.variants).not.toBe(gpt55Fast.variants)
    expect(Object.keys(gpt55Variants).some((variant) => variant.endsWith('-fast'))).toBe(false)
    expect(gpt54Fast.options.serviceTier).not.toBe('wrong-from-raw')
    expect(gpt54Fast.headers['x-test-header']).not.toBe('wrong-raw-header')
    expect(gpt54FastSys.options.serviceTier).not.toBe('wrong-from-raw')
    expect(gpt54FastSys.headers['x-test-header']).not.toBe('wrong-raw-header')
    expect(modelsJson).not.toContain('"experimental"')
    expect(modelsJson).not.toContain('"provider"')
    expect(modelsJson).not.toContain('"tools"')
    expect(parsed.agent.build.options.store).toBe(false)
    expect(parsed.agent.plan.options.store).toBe(false)
    expect(parsed.agent.image).toEqual({
      mode: 'subagent',
      description: expect.stringContaining('Generate images with GPT-5.5 Image Fast (Sys)'),
      model: 'sub2api-openai/gpt-5.5-fast-Sys',
      variant: 'image',
      options: { store: false }
    })
    expect(models[parsed.agent.image.model.replace('sub2api-openai/', '')].variants.image).toBeDefined()
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
