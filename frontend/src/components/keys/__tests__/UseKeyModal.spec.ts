import { beforeEach, describe, expect, it, vi } from 'vitest'
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
    t: (key: string, params?: Record<string, string>) => {
      if (params?.url) return `${key} ${params.url}`
      if (key === 'keys.useKeyModal.agentConfig.hint') return '逐项下载到本地临时副本；配置文件不存在时直接复制；已有配置时先对比合并；完成后先检查。'
      return key
    }
  })
}))

vi.mock('@/composables/useClipboard', () => ({
  useClipboard: () => ({
    copyToClipboard: vi.fn().mockResolvedValue(true)
  })
}))

vi.mock('@/api/keys', () => ({
  buildAgentConfigGuidePath: (client: 'omp' | 'opencode', apiKey: string, baseUrl?: string) => {
    const clientPath = client === 'omp' ? 'omp-openai' : 'opencode-openai'
    const params = new URLSearchParams()
    params.set('api_key', apiKey)
    const trimmedBaseURL = baseUrl?.trim()
    if (trimmedBaseURL) params.set('base_url', trimmedBaseURL)
    return `/config-guides/${clientPath}/manifest.json?${params.toString()}`
  },
  keysAPI: {
    getOpenCodeOpenAIModels: vi.fn().mockResolvedValue({
      models: openaiModelsMock,
      omp_openai_provider_tools: {
        package: 'omp-openai-provider-tools',
        latest_version: '0.1.2',
        status: 'ok'
      }
    })
  }
}))

import { keysAPI } from '@/api/keys'

import UseKeyModal from '../UseKeyModal.vue'

const defaultOpenCodeOpenAIModelsResponse = () => ({
  models: openaiModelsMock,
  omp_openai_provider_tools: {
    package: 'omp-openai-provider-tools',
    latest_version: '0.1.2',
    status: 'ok' as const
  }
})

type UseKeyModalProps = {
  show: boolean
  apiKey: string
  baseUrl: string
  platform: 'openai'
  allowMessagesDispatch?: boolean
}

const mountOpenAIUseKeyModal = (props: Partial<UseKeyModalProps> = {}) => mount(UseKeyModal, {
  props: {
    show: true,
    apiKey: 'sk-test',
    baseUrl: 'https://example.com/v1',
    platform: 'openai',
    ...props
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

type UseKeyModalWrapper = ReturnType<typeof mountOpenAIUseKeyModal>

const flushModalAsync = async () => {
  await nextTick()
  await Promise.resolve()
  await nextTick()
}

const clickClientTab = async (wrapper: UseKeyModalWrapper, label: string) => {
  const tab = wrapper.findAll('button').find((button) => button.text().includes(label))
  expect(tab).toBeDefined()
  await tab!.trigger('click')
  await flushModalAsync()
}

const getCodeBlocks = (wrapper: UseKeyModalWrapper) => wrapper.findAll('pre code').map((code) => code.text())

const getOpenCodeModelsMock = () => vi.mocked(keysAPI.getOpenCodeOpenAIModels)

describe('UseKeyModal', () => {
  beforeEach(() => {
    const getModels = getOpenCodeModelsMock()
    getModels.mockReset()
    getModels.mockResolvedValue(defaultOpenCodeOpenAIModelsResponse())
  })

  it('renders GPT-5.4 small-model entries in OpenCode config', async () => {
    const wrapper = mountOpenAIUseKeyModal()

    await clickClientTab(wrapper, 'keys.useKeyModal.cliTabs.opencode')

    const codeBlock = wrapper.find('pre code')
    expect(codeBlock.exists()).toBe(true)
    expect(codeBlock.text()).toContain('"name": "GPT-5.4 Mini"')
    expect(codeBlock.text()).toContain('"name": "GPT-5.4 Nano"')
  })

  it('renders sub2api-openai provider config with Sys models in OpenCode example', async () => {
    const wrapper = mountOpenAIUseKeyModal()

    await clickClientTab(wrapper, 'keys.useKeyModal.cliTabs.opencode')

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

  it('shows OMP tab with OMP-specific description and loads OpenAI metadata', async () => {
    const getModels = getOpenCodeModelsMock()
    getModels.mockClear()
    const wrapper = mountOpenAIUseKeyModal()

    await clickClientTab(wrapper, 'keys.useKeyModal.cliTabs.omp')

    expect(getModels).toHaveBeenCalledTimes(1)
    const description = wrapper.find('p.text-sm.text-gray-600').text()
    expect(description).toContain('keys.useKeyModal.omp.description')
    expect(description).not.toContain('Codex')
    expect(description).not.toContain('.codex')
    expect(description).not.toContain('keys.useKeyModal.openai.description')
    expect(wrapper.text()).not.toContain('keys.useKeyModal.openai.note')
  })

  it('renders OMP plugin, models.yml, and config.yml blocks with dynamic plugin version', async () => {
    const getModels = getOpenCodeModelsMock()
    getModels.mockResolvedValueOnce({
      models: openaiModelsMock,
      omp_openai_provider_tools: {
        package: 'omp-openai-provider-tools',
        latest_version: '9.9.9',
        status: 'ok'
      }
    })
    const wrapper = mountOpenAIUseKeyModal()

    await clickClientTab(wrapper, 'keys.useKeyModal.cliTabs.omp')

    const codeBlocks = getCodeBlocks(wrapper)
    expect(codeBlocks).toHaveLength(3)
    expect(wrapper.text()).toContain('1. Install OMP provider tools plugin')
    expect(wrapper.text()).toContain('~/.omp/agent/models.yml')
    expect(wrapper.text()).toContain('~/.omp/agent/config.yml')
    expect(wrapper.text().indexOf('1. Install OMP provider tools plugin')).toBeLessThan(wrapper.text().indexOf('~/.omp/agent/models.yml'))
    expect(wrapper.text().indexOf('~/.omp/agent/models.yml')).toBeLessThan(wrapper.text().indexOf('~/.omp/agent/config.yml'))

    expect(codeBlocks[0]).toContain('omp plugin install npm:omp-openai-provider-tools@9.9.9')
    expect(codeBlocks[0]).toContain('omp plugin doctor')
    expect(codeBlocks[0]).toContain('npx omp-openai-provider-tools configure-image-agent --model sub2api-openai-image/gpt-5.5-Sys --dry-run')
    expect(codeBlocks[0]).toContain('npx omp-openai-provider-tools configure-image-agent --model sub2api-openai-image/gpt-5.5-Sys')
    expect(codeBlocks[0]).toContain('--print')
    expect(codeBlocks[0]).not.toContain('omp-openai-provider-tools@latest')
    expect(codeBlocks[0]).not.toContain('omp plugin install npm:omp-openai-provider-tools\n')
    expect(codeBlocks[0]).not.toContain('omp-openai-provider-tools@0.1.2')

    expect(codeBlocks[1]).toContain('providers:')
    expect(codeBlocks[1]).toContain('sub2api-openai:')
    expect(codeBlocks[1]).toContain('sub2api-openai-image:')
    expect(codeBlocks[1]).toContain('api: openai-responses')
    expect(codeBlocks[1]).toContain('baseUrl: https://example.com/v1')
    expect(codeBlocks[1]).toContain('apiKey: sk-test')
    expect(codeBlocks[1]).toContain('openaiProviderTools:')
    expect(codeBlocks[1]).toContain('enabled: true')
    expect(codeBlocks[1]).toContain('imageGeneration: true')
    expect(codeBlocks[1]).toContain('sub2api-openai-image/gpt-5.5-Sys: gpt-5.5-image-sys')
    expect(codeBlocks[1]).not.toContain('attachment')
    expect(codeBlocks[1]).not.toContain('tool_call')
    expect(codeBlocks[1]).not.toContain('structured_output')
    expect(codeBlocks[1]).not.toContain('temperature')
    expect(codeBlocks[1]).not.toContain('release_date')
    expect(codeBlocks[1]).not.toContain('variants')
    expect(codeBlocks[1]).not.toContain('modalities')
    expect(codeBlocks[1]).not.toContain('pdf')
    expect(codeBlocks[1]).not.toContain('experimental')
    expect(codeBlocks[1]).not.toMatch(/^\s+provider:/m)
    expect(codeBlocks[1]).not.toMatch(/^\s+tools:/m)

    const ordinaryProvider = codeBlocks[1].slice(
      codeBlocks[1].indexOf('  sub2api-openai:'),
      codeBlocks[1].indexOf('  sub2api-openai-image:')
    )
    expect(ordinaryProvider).not.toContain('imageGeneration: true')

    expect(codeBlocks[2]).toContain('default: sub2api-openai/gpt-5.5-Sys')
    expect(codeBlocks[2]).toContain('smol: sub2api-openai/gpt-5.4-mini-Sys')
    expect(codeBlocks[2]).toContain('plan: sub2api-openai/gpt-5.5-Sys')
    expect(codeBlocks[2]).toContain('task: sub2api-openai/gpt-5.5-Sys:xhigh')
    expect(codeBlocks[2]).not.toContain('task: gpt-5.5-sys:xhigh')
    expect(codeBlocks[2]).not.toContain('smol: gpt-5.4-mini-sys')
  })

  it('does not render a half plugin install command when OMP plugin version is unavailable', async () => {
    getOpenCodeModelsMock().mockResolvedValueOnce({
      models: openaiModelsMock,
      omp_openai_provider_tools: {
        package: 'omp-openai-provider-tools',
        latest_version: '',
        status: 'unavailable',
        error: 'npm registry unavailable'
      }
    })
    const wrapper = mountOpenAIUseKeyModal()

    await clickClientTab(wrapper, 'keys.useKeyModal.cliTabs.omp')

    const text = wrapper.text()
    const combinedBlocks = getCodeBlocks(wrapper).join('\n')
    expect(text).toContain('keys.useKeyModal.omp.pluginVersionErrorHint')
    expect(combinedBlocks).not.toContain('omp plugin install npm:omp-openai-provider-tools@')
    expect(combinedBlocks).not.toContain('providers:')
  })

  it('does not render OMP success configs when a required role model is missing', async () => {
    getOpenCodeModelsMock().mockResolvedValueOnce({
      models: {
        'gpt-5.5': openaiModelsMock['gpt-5.5']
      },
      omp_openai_provider_tools: {
        package: 'omp-openai-provider-tools',
        latest_version: '0.1.2',
        status: 'ok'
      }
    })
    const wrapper = mountOpenAIUseKeyModal()

    await clickClientTab(wrapper, 'keys.useKeyModal.cliTabs.omp')

    const combinedBlocks = getCodeBlocks(wrapper).join('\n')
    expect(wrapper.text()).toContain('keys.useKeyModal.omp.metadataErrorHint')
    expect(combinedBlocks).not.toContain('providers:')
    expect(combinedBlocks).not.toContain('smol: sub2api-openai/gpt-5.4-mini-Sys')
    expect(combinedBlocks).not.toContain('apiKey: sk-test')
  })

  it('refetches OMP metadata after loading legacy OpenCode metadata without plugin details', async () => {
    const getModels = getOpenCodeModelsMock()
    getModels
      .mockResolvedValueOnce({ models: openaiModelsMock })
      .mockResolvedValueOnce(defaultOpenCodeOpenAIModelsResponse())
    const wrapper = mountOpenAIUseKeyModal()

    await clickClientTab(wrapper, 'keys.useKeyModal.cliTabs.opencode')
    expect(getModels).toHaveBeenCalledTimes(1)

    await clickClientTab(wrapper, 'keys.useKeyModal.cliTabs.omp')

    expect(getModels).toHaveBeenCalledTimes(2)
    expect(getCodeBlocks(wrapper).join('\n')).toContain('omp plugin install npm:omp-openai-provider-tools@0.1.2')
  })

  it('preserves cached OpenCode models when OMP metadata refetch fails after a legacy response', async () => {
    const getModels = getOpenCodeModelsMock()
    getModels
      .mockRejectedValue(new Error('unexpected refetch'))
      .mockResolvedValueOnce({ models: openaiModelsMock })
      .mockRejectedValueOnce(new Error('npm down'))
    const wrapper = mountOpenAIUseKeyModal()

    await clickClientTab(wrapper, 'keys.useKeyModal.cliTabs.opencode')
    expect(getModels).toHaveBeenCalledTimes(1)
    let codeBlocks = getCodeBlocks(wrapper)
    expect(codeBlocks.join('\n')).toContain('"sub2api-openai"')
    expect(codeBlocks.join('\n')).toContain('"gpt-5.4"')

    await clickClientTab(wrapper, 'keys.useKeyModal.cliTabs.omp')
    expect(getModels).toHaveBeenCalledTimes(2)
    codeBlocks = getCodeBlocks(wrapper)
    expect(wrapper.text()).toContain('Failed to load OpenCode OpenAI metadata')
    expect(codeBlocks.join('\n')).not.toContain('providers:')

    await clickClientTab(wrapper, 'keys.useKeyModal.cliTabs.opencode')
    expect(getModels).toHaveBeenCalledTimes(2)
    codeBlocks = getCodeBlocks(wrapper)
    expect(codeBlocks.join('\n')).toContain('"sub2api-openai"')
    expect(codeBlocks.join('\n')).toContain('"gpt-5.4"')
    expect(wrapper.find('[data-testid="agent-config-guide"]').exists()).toBe(true)
    expect(wrapper.text()).toContain('/config-guides/opencode-openai/manifest.json?api_key=sk-test')
  })

  it('does not render OMP provider YAML when metadata loading fails', async () => {
    getOpenCodeModelsMock().mockRejectedValueOnce(new Error('network down'))
    const wrapper = mountOpenAIUseKeyModal()

    await clickClientTab(wrapper, 'keys.useKeyModal.cliTabs.omp')

    const combinedBlocks = getCodeBlocks(wrapper).join('\n')
    expect(wrapper.text()).toContain('Failed to load OpenCode OpenAI metadata')
    expect(combinedBlocks).not.toContain('providers:')
    expect(combinedBlocks).not.toContain('apiKey: sk-test')
  })

  it('shows short OMP agent config guide link without base_url by default', async () => {
    const wrapper = mountOpenAIUseKeyModal()

    await clickClientTab(wrapper, 'keys.useKeyModal.cliTabs.omp')

    const text = wrapper.text()
    expect(text).toContain('keys.useKeyModal.agentConfig.instruction')
    expect(text).toContain('/config-guides/omp-openai/manifest.json?api_key=sk-test')
    expect(text).not.toContain('base_url=')

    expect(text).toContain('逐项下载')
    expect(text).toContain('本地临时副本')
    expect(text).toContain('已有配置时先对比合并')
    expect(text).toContain('完成后先检查')
    const agentBlock = wrapper.find('[data-testid="agent-config-guide"]')
    expect(agentBlock.exists()).toBe(true)
    expect(agentBlock.text()).not.toContain('providers:')
    expect(getCodeBlocks(wrapper)).toHaveLength(3)
  })

  it('uses api base origin instead of window origin for the agent link', async () => {
    const wrapper = mountOpenAIUseKeyModal({ baseUrl: 'https://example.com/v1' })

    await clickClientTab(wrapper, 'keys.useKeyModal.cliTabs.omp')

    expect(wrapper.text()).toContain('https://example.com/config-guides/omp-openai/manifest.json?api_key=sk-test')
  })

  it('falls back to the page origin when api base url is relative', async () => {
    const wrapper = mountOpenAIUseKeyModal({ baseUrl: '/api/v1' })

    await clickClientTab(wrapper, 'keys.useKeyModal.cliTabs.omp')

    expect(wrapper.text()).toContain('/config-guides/omp-openai/manifest.json?api_key=sk-test')
    expect(wrapper.text()).not.toContain('/api/config-guides/')
  })

  it('hides OMP agent config guide link when required model metadata is missing', async () => {
    getOpenCodeModelsMock().mockResolvedValueOnce({
      models: { 'gpt-5.5': openaiModelsMock['gpt-5.5'] },
      omp_openai_provider_tools: {
        package: 'omp-openai-provider-tools',
        latest_version: '0.1.2',
        status: 'ok'
      }
    })
    const wrapper = mountOpenAIUseKeyModal()

    await clickClientTab(wrapper, 'keys.useKeyModal.cliTabs.omp')

    expect(wrapper.find('[data-testid="agent-config-guide"]').exists()).toBe(false)
    expect(wrapper.text()).not.toContain('/config-guides/omp-openai/manifest.json')
  })

  it('hides OpenCode agent config guide link when required model metadata is incomplete', async () => {
    getOpenCodeModelsMock().mockResolvedValueOnce({
      models: { 'gpt-5.5': openaiModelsMock['gpt-5.5'] },
      omp_openai_provider_tools: {
        package: 'omp-openai-provider-tools',
        latest_version: '0.1.2',
        status: 'ok'
      }
    })
    const wrapper = mountOpenAIUseKeyModal()

    await clickClientTab(wrapper, 'keys.useKeyModal.cliTabs.opencode')

    expect(wrapper.find('[data-testid="agent-config-guide"]').exists()).toBe(false)
    expect(wrapper.text()).not.toContain('/config-guides/opencode-openai/manifest.json')
  })

  it('shows short OpenCode agent config guide link when OpenCode metadata is loaded', async () => {
    const wrapper = mountOpenAIUseKeyModal()

    await clickClientTab(wrapper, 'keys.useKeyModal.cliTabs.opencode')

    expect(wrapper.text()).toContain('/config-guides/opencode-openai/manifest.json?api_key=sk-test')
    expect(wrapper.text()).not.toContain('base_url=')
    expect(getCodeBlocks(wrapper)).toHaveLength(1)
    expect(getCodeBlocks(wrapper).join('\n')).toContain('"sub2api-openai"')
  })

  it('hides OMP agent config guide link when OMP metadata loading fails', async () => {
    getOpenCodeModelsMock().mockRejectedValueOnce(new Error('network down'))
    const wrapper = mountOpenAIUseKeyModal()

    await clickClientTab(wrapper, 'keys.useKeyModal.cliTabs.omp')

    expect(wrapper.find('[data-testid="agent-config-guide"]').exists()).toBe(false)
    expect(wrapper.text()).not.toContain('/config-guides/omp-openai/manifest.json')
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
