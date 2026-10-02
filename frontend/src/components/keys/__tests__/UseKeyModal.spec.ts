import { afterEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import { nextTick } from 'vue'

const { copyToClipboardMock, saveAsMock } = vi.hoisted(() => ({
  copyToClipboardMock: vi.fn().mockResolvedValue(true),
  saveAsMock: vi.fn()
}))

vi.mock('vue-i18n', () => ({
  useI18n: () => ({
    t: (key: string) => key
  })
}))

vi.mock('@/composables/useClipboard', () => ({
  useClipboard: () => ({
    copyToClipboard: copyToClipboardMock
  })
}))

vi.mock('file-saver', () => ({
  saveAs: saveAsMock
}))

import UseKeyModal from '../UseKeyModal.vue'

function readBlobAsText(blob: Blob): Promise<string> {
  return new Promise((resolve, reject) => {
    const reader = new FileReader()
    reader.addEventListener('load', () => resolve(String(reader.result || '')))
    reader.addEventListener('error', () => reject(reader.error))
    reader.readAsText(blob)
  })
}

describe('UseKeyModal', () => {
  afterEach(() => {
    vi.unstubAllGlobals()
    saveAsMock.mockClear()
  })

  it('shows only Claude Code for Claude Code-only groups', async () => {
    const wrapper = mount(UseKeyModal, {
      props: {
        show: true,
        apiKey: 'sk-anthropic-test',
        baseUrl: 'https://example.com/v1',
        platform: 'anthropic'
      },
      global: {
        stubs: {
          BaseDialog: { template: '<div><slot /><slot name="footer" /></div>' },
          Icon: { template: '<span />' }
        }
      }
    })

    const clientTabs = () => wrapper.find('nav[aria-label="Client"]').text()
    expect(clientTabs()).toContain('keys.useKeyModal.cliTabs.codexCli')
    expect(clientTabs()).toContain('keys.useKeyModal.cliTabs.opencode')

    const codexTab = wrapper.find('nav[aria-label="Client"]').findAll('button').find(
      (button) => button.text().includes('keys.useKeyModal.cliTabs.codexCli')
    )
    await codexTab!.trigger('click')
    await wrapper.setProps({ claudeCodeOnly: true })

    expect(clientTabs()).toContain('keys.useKeyModal.cliTabs.claudeCode')
    expect(clientTabs()).not.toContain('keys.useKeyModal.cliTabs.codexCli')
    expect(clientTabs()).not.toContain('keys.useKeyModal.cliTabs.opencode')
    expect(wrapper.find('pre code').text()).toContain('CLAUDE_CODE_DISABLE_NONESSENTIAL_TRAFFIC')

    await wrapper.setProps({ platform: 'openai' })
    expect(clientTabs()).toContain('keys.useKeyModal.cliTabs.claudeCode')
    expect(clientTabs()).not.toContain('keys.useKeyModal.cliTabs.codexCli')
    expect(clientTabs()).not.toContain('keys.useKeyModal.cliTabs.opencode')
    expect(wrapper.find('pre code').text()).toContain('ANTHROPIC_BASE_URL')
  })

  it('omits the attribution override from every standard Claude Code setup form', async () => {
    const wrapper = mount(UseKeyModal, {
      props: {
        show: true,
        apiKey: 'sk-anthropic-test',
        baseUrl: 'https://example.com/v1',
        platform: 'anthropic'
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

    for (const [shell, trafficSetting] of [
      ['macOS / Linux', 'export CLAUDE_CODE_DISABLE_NONESSENTIAL_TRAFFIC=1'],
      ['Windows CMD', 'set CLAUDE_CODE_DISABLE_NONESSENTIAL_TRAFFIC=1'],
      ['PowerShell', '$env:CLAUDE_CODE_DISABLE_NONESSENTIAL_TRAFFIC=1']
    ]) {
      if (shell !== 'macOS / Linux') {
        const shellTab = wrapper.findAll('button').find(
          (button) => button.text().trim() === shell
        )
        expect(shellTab).toBeDefined()
        await shellTab!.trigger('click')
        await nextTick()
      }

      const codeBlocks = wrapper.findAll('pre code').map((code) => code.text())
      const allCode = codeBlocks.join('\n')
      const settings = JSON.parse(codeBlocks.find((content) => content.includes('"$schema"'))!)

      expect(allCode).not.toContain('CLAUDE_CODE_ATTRIBUTION_HEADER')
      expect(allCode).toContain(trafficSetting)
      expect(settings.env.CLAUDE_CODE_DISABLE_NONESSENTIAL_TRAFFIC).toBe('1')
      expect(settings.env).not.toHaveProperty('CLAUDE_CODE_ATTRIBUTION_HEADER')
    }
  })

  it('renders Grok Build and OpenCode setup for Grok groups', async () => {
    const wrapper = mount(UseKeyModal, {
      props: {
        show: true,
        apiKey: 'sk-grok-test',
        baseUrl: 'https://example.com/v1',
        platform: 'grok'
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

    const grokTab = wrapper.findAll('button').find((button) =>
      button.text().includes('keys.useKeyModal.cliTabs.grokCli')
    )
    expect(grokTab).toBeDefined()

    const allCode = wrapper.findAll('pre code').map((code) => code.text()).join('\n')
    expect(allCode).toContain('GROK_MODELS_BASE_URL')
    expect(allCode).toContain('XAI_API_KEY')
    expect(allCode).toContain('[model."grok-4.5"]')
    expect(allCode).toContain('[model."grok-build-0.1"]')
    expect(allCode).toContain('[model."grok-4.20-multi-agent-0309"]')
    expect(allCode).toContain('[model."grok-4.3"]')
    expect(allCode).toContain('default = "grok-4.5"')
    expect(allCode).toContain('models_base_url = "https://example.com/v1"')
    expect(allCode).toContain('models_list_url = "https://example.com/v1/models"')
    expect(allCode).toContain('xai_api_base_url = "https://example.com/v1"')
    expect(allCode).toContain('cli_chat_proxy_base_url = "https://example.com/v1"')
    expect(allCode).toContain('preferred_method = "api_key"')
    expect(allCode).toContain('image_description = "grok-4.5"')
    expect(allCode).toContain('auto_compact_threshold_percent = 80')
    expect(allCode).toContain('image_gen = true')
    expect(allCode).toContain('video_gen = true')
    expect(allCode).toContain('image_gen_model_override = "grok-imagine-image-quality"')
    expect(allCode).toContain('image_edit_model_override = "grok-imagine-edit"')
    expect(allCode).toContain('env_key = "XAI_API_KEY"')
    expect(allCode).toContain('Keep api_backend = "responses" on every model entry.')
    expect(allCode).toContain('grok-imagine-image')
    expect(allCode).toContain('grok-imagine-edit')
    expect(allCode).toMatch(/\[model\."grok-4\.5"\][\s\S]*?context_window = 500000/)
    expect(allCode).toMatch(/\[model\."grok-build-0\.1"\][\s\S]*?context_window = 256000/)
    // Prefer env_key; hardcode api_key only as commented alternative
    expect(allCode).not.toMatch(/^api_key = "sk-grok-test"$/m)

    const modelBlocks = allCode
      .split(/(?=^\[model\.)/m)
      .filter((block) => block.startsWith('[model."'))
    expect(modelBlocks.length).toBeGreaterThanOrEqual(4)
    for (const block of modelBlocks) {
      if (block.includes('# [model.')) continue
      expect(block).toContain('api_backend = "responses"')
    }

    const windowsTab = wrapper.findAll('button').find(
      (button) => button.text().trim() === 'Windows'
    )
    expect(windowsTab).toBeDefined()
    await windowsTab!.trigger('click')
    await nextTick()
    expect(wrapper.text().toLowerCase()).toContain('%userprofile%\\.grok\\config.toml')

    const opencodeTab = wrapper.findAll('button').find((button) =>
      button.text().includes('keys.useKeyModal.cliTabs.opencode')
    )
    expect(opencodeTab).toBeDefined()
    await opencodeTab!.trigger('click')
    await nextTick()

    const parsed = JSON.parse(wrapper.find('pre code').text())
    expect(parsed.provider.grok.npm).toBe('@ai-sdk/openai-compatible')
    expect(parsed.provider.grok.name).toBe('Grok via Sub2API')
    expect(parsed.provider.grok.options).toEqual({
      baseURL: 'https://example.com/v1',
      apiKey: 'sk-grok-test'
    })
    expect(parsed.provider.grok.models['grok-4.5']).toBeDefined()
    expect(parsed.provider.grok.models['grok-4.5'].limit.context).toBe(500000)
    expect(parsed.provider.grok.models['grok-build-0.1']).toBeDefined()
    expect(parsed.provider.grok.models['grok-4.20-multi-agent-0309']).toBeDefined()
    expect(parsed.provider.grok.models['grok-composer-2.5-fast']).toBeDefined()
    expect(parsed.provider.grok.models['gpt-5.6']).toBeUndefined()
  })

  it('renders copyable Claude Code setup through the Grok Messages gateway', async () => {
    copyToClipboardMock.mockClear()
    const wrapper = mount(UseKeyModal, {
      props: {
        show: true,
        apiKey: 'sk-grok-claude-test',
        baseUrl: 'https://example.com/v1',
        platform: 'grok'
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

    const claudeTab = wrapper.findAll('button').find((button) =>
      button.text().includes('keys.useKeyModal.cliTabs.claudeCode')
    )
    expect(claudeTab).toBeDefined()
    await claudeTab!.trigger('click')
    await nextTick()

    let codeBlocks = wrapper.findAll('pre code').map((code) => code.text())
    expect(codeBlocks.join('\n')).toContain('ANTHROPIC_BASE_URL="https://example.com"')
    expect(codeBlocks.join('\n')).toContain('ANTHROPIC_AUTH_TOKEN="sk-grok-claude-test"')
    const unixConfig = codeBlocks.find((content) => content.startsWith('export ANTHROPIC_BASE_URL'))
    expect(unixConfig).toBeDefined()
    for (const name of [
      'ANTHROPIC_MODEL',
      'ANTHROPIC_DEFAULT_OPUS_MODEL',
      'ANTHROPIC_DEFAULT_SONNET_MODEL',
      'ANTHROPIC_DEFAULT_HAIKU_MODEL',
      'ANTHROPIC_DEFAULT_FABLE_MODEL',
      'CLAUDE_CODE_SUBAGENT_MODEL'
    ]) {
      expect(unixConfig).toContain(`export ${name}="grok-4.5"`)
    }
    const settingsConfig = codeBlocks.find((content) => content.includes('"$schema"'))
    expect(settingsConfig).toBeDefined()
    const parsedSettings = JSON.parse(settingsConfig!)
    expect(parsedSettings.$schema).toBe('https://json.schemastore.org/claude-code-settings.json')
    expect(parsedSettings.env.ANTHROPIC_MODEL).toBe('grok-4.5')
    expect(codeBlocks.join('\n')).not.toContain('CLAUDE_CODE_ATTRIBUTION_HEADER')
    expect(codeBlocks.join('\n')).toContain('CLAUDE_CODE_DISABLE_NONESSENTIAL_TRAFFIC')
    expect(parsedSettings.env.CLAUDE_CODE_DISABLE_NONESSENTIAL_TRAFFIC).toBe('1')
    expect(parsedSettings.env).not.toHaveProperty('CLAUDE_CODE_ATTRIBUTION_HEADER')
    expect(wrapper.text()).toContain('keys.useKeyModal.claudeSettingsHint')
    expect(wrapper.text()).toContain('keys.useKeyModal.grok.claudeNote')
    expect(wrapper.find('nav[aria-label="Client"]').classes()).toContain('min-w-max')
    expect(wrapper.find('nav[aria-label="Client"]').element.parentElement?.classList.contains('overflow-x-auto')).toBe(true)

    const cmdTab = wrapper.findAll('button').find(
      (button) => button.text().trim() === 'Windows CMD'
    )
    expect(cmdTab).toBeDefined()
    await cmdTab!.trigger('click')
    await nextTick()

    codeBlocks = wrapper.findAll('pre code').map((code) => code.text())
    expect(codeBlocks.join('\n')).toContain('set ANTHROPIC_MODEL=grok-4.5')
    expect(codeBlocks.join('\n')).toContain('set ANTHROPIC_DEFAULT_FABLE_MODEL=grok-4.5')
    expect(codeBlocks.join('\n')).toContain('set CLAUDE_CODE_SUBAGENT_MODEL=grok-4.5')
    expect(codeBlocks.join('\n')).toContain('set CLAUDE_CODE_DISABLE_NONESSENTIAL_TRAFFIC=1')
    expect(codeBlocks.join('\n')).not.toContain('CLAUDE_CODE_ATTRIBUTION_HEADER')
    const cmdSettings = JSON.parse(codeBlocks.find((content) => content.includes('"$schema"'))!)
    expect(cmdSettings.env.CLAUDE_CODE_DISABLE_NONESSENTIAL_TRAFFIC).toBe('1')
    expect(cmdSettings.env).not.toHaveProperty('CLAUDE_CODE_ATTRIBUTION_HEADER')

    const powershellTab = wrapper.findAll('button').find(
      (button) => button.text().trim() === 'PowerShell'
    )
    expect(powershellTab).toBeDefined()
    await powershellTab!.trigger('click')
    await nextTick()

    codeBlocks = wrapper.findAll('pre code').map((code) => code.text())
    expect(codeBlocks.join('\n')).toContain('$env:ANTHROPIC_BASE_URL="https://example.com"')
    expect(codeBlocks.join('\n')).toContain('$env:ANTHROPIC_MODEL="grok-4.5"')
    expect(codeBlocks.join('\n')).toContain('$env:ANTHROPIC_DEFAULT_FABLE_MODEL="grok-4.5"')
    expect(codeBlocks.join('\n')).toContain('$env:CLAUDE_CODE_SUBAGENT_MODEL="grok-4.5"')
    expect(codeBlocks.join('\n')).toContain('$env:CLAUDE_CODE_DISABLE_NONESSENTIAL_TRAFFIC="1"')
    expect(codeBlocks.join('\n')).not.toContain('CLAUDE_CODE_ATTRIBUTION_HEADER')
    const powershellSettings = JSON.parse(codeBlocks.find((content) => content.includes('"$schema"'))!)
    expect(powershellSettings.env.CLAUDE_CODE_DISABLE_NONESSENTIAL_TRAFFIC).toBe('1')
    expect(powershellSettings.env).not.toHaveProperty('CLAUDE_CODE_ATTRIBUTION_HEADER')
    expect(wrapper.text()).toContain('%USERPROFILE%\\.claude\\settings.json')

    const copyButton = wrapper.findAll('button').find((button) =>
      button.text().includes('keys.useKeyModal.copy')
    )
    expect(copyButton).toBeDefined()
    await copyButton!.trigger('click')
    expect(copyToClipboardMock).toHaveBeenCalledWith(
      expect.stringContaining('ANTHROPIC_AUTH_TOKEN="sk-grok-claude-test"'),
      'keys.copied'
    )
  })

  it('renders Codex custom provider setup through the Grok Responses gateway', async () => {
    const wrapper = mount(UseKeyModal, {
      props: {
        show: true,
        apiKey: 'sk-grok-codex-test',
        baseUrl: 'https://example.com/v1',
        platform: 'grok'
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

    const codexTab = wrapper.findAll('button').find((button) =>
      button.text().includes('keys.useKeyModal.cliTabs.codexCli')
    )
    expect(codexTab).toBeDefined()
    await codexTab!.trigger('click')
    await nextTick()

    let codeBlocks = wrapper.findAll('pre code').map((code) => code.text())
    const configToml = codeBlocks.find((content) => content.includes('[model_providers.sub2api]'))
    expect(configToml).toBeDefined()
    expect(configToml).toContain('model_provider = "sub2api"')
    expect(configToml).toContain('model = "grok-4.5"')
    expect(configToml).toContain('base_url = "https://example.com/v1"')
    expect(configToml).toContain('env_key = "SUB2API_API_KEY"')
    expect(configToml).toContain('wire_api = "responses"')
    // API-key provider: Codex must not require a ChatGPT OAuth login.
    expect(configToml).toContain('requires_openai_auth = false')
    expect(configToml).toContain('supports_websockets = false')
    expect(configToml).toContain('grok-4.20-multi-agent-0309 (text / web_search)')
    expect(configToml).toContain('grok-imagine-image')
    expect(configToml).toContain('grok-imagine-video')
    // Hardcoded bearer is only a commented fallback when env cannot be set.
    expect(configToml).toMatch(/# experimental_bearer_token = "sk-grok-codex-test"/)
    expect(configToml).not.toContain('supports_websockets = true')
    expect(configToml).not.toContain('responses_websockets_v2')
    expect(wrapper.text()).not.toContain('auth.json')
    expect(codeBlocks.join('\n')).toContain('SUB2API_API_KEY')

    const windowsTab = wrapper.findAll('button').find(
      (button) => button.text().trim() === 'Windows'
    )
    expect(windowsTab).toBeDefined()
    await windowsTab!.trigger('click')
    await nextTick()

    codeBlocks = wrapper.findAll('pre code').map((code) => code.text())
    expect(wrapper.text().toLowerCase()).toContain('%userprofile%\\.codex\\config.toml'.toLowerCase())
    expect(codeBlocks.join('\n')).toContain('experimental_bearer_token = "sk-grok-codex-test"')
  })

  it('keeps legacy OpenAI Codex config as the default', () => {
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

    const codeBlocks = wrapper.findAll('pre code').map((code) => code.text())
    const configToml = codeBlocks.find((content) => content.includes('model_provider = "OpenAI"'))

    expect(configToml).toBeDefined()
    expect(configToml).toContain('model = "gpt-5.5"')
    expect(configToml).toContain('review_model = "gpt-5.5"')
    expect(configToml).not.toContain('model = "gpt-5.4"')
    expect(configToml).not.toContain('model_context_window')
    expect(configToml).not.toContain('model_auto_compact_token_limit')
    expect(configToml).toContain('requires_openai_auth = true')
    expect(configToml).not.toContain('experimental_bearer_token')
    expect(configToml).not.toContain('x-openai-actor-authorization')
    expect(configToml).not.toContain('env_key')
    expect(configToml).not.toContain('image_generation')
    expect(configToml).not.toContain('supports_websockets')
    expect(configToml).not.toContain('responses_websockets_v2')
    expect(configToml).toContain('[features]\ngoals = true')
    expect(configToml).not.toContain('model_reasoning_effort = "xhigh"')
    expect(codeBlocks).toContain('{\n  "OPENAI_API_KEY": "sk-test"\n}')
    expect(wrapper.text()).toContain('auth.json')
    expect(wrapper.find('[data-testid="codex-api-key-restart-notice"]').exists()).toBe(false)
  })

  it('normalizes OpenAI Codex and Claude base URLs independently', async () => {
    const wrapper = mount(UseKeyModal, {
      props: {
        show: true,
        apiKey: 'sk-test',
        baseUrl: 'https://example.com',
        platform: 'openai',
        allowMessagesDispatch: true
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

    const codexConfig = wrapper.findAll('pre code')
      .map((code) => code.text())
      .find((content) => content.includes('model_provider = "OpenAI"'))
    expect(codexConfig).toContain('base_url = "https://example.com/v1"')

    const claudeTab = wrapper.findAll('button').find((button) =>
      button.text().includes('keys.useKeyModal.cliTabs.claudeCode')
    )
    expect(claudeTab).toBeDefined()
    await claudeTab!.trigger('click')
    await nextTick()

    const claudeConfig = wrapper.findAll('pre code')
      .map((code) => code.text())
      .find((content) => content.startsWith('export ANTHROPIC_BASE_URL'))
    expect(claudeConfig).toContain('ANTHROPIC_BASE_URL="https://example.com"')
  })

  it('renders API Key Mode authorization in OpenAI Codex config', async () => {
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

    const apiKeyMode = wrapper.get('[data-testid="codex-auth-mode-api-key"]')
    await apiKeyMode.trigger('click')
    await nextTick()

    const codeBlocks = wrapper.findAll('pre code').map((code) => code.text())
    const configToml = codeBlocks.find((content) => content.includes('model_provider = "OpenAI"'))

    expect(apiKeyMode.attributes('aria-checked')).toBe('true')
    expect(configToml).toBeDefined()
    expect(configToml).toContain('requires_openai_auth = false')
    expect(configToml).toContain('experimental_bearer_token = "sk-test"')
    expect(configToml).toContain('http_headers = { "x-openai-actor-authorization" = "local-image-extension" }')
    expect(configToml).toContain('model_catalog_url = "https://example.com/v1/models"')
    expect(configToml).not.toContain('model_catalog_json')
    expect(configToml).not.toContain('env_key')
    expect(configToml).not.toContain('image_generation')
    expect(codeBlocks).not.toContain('{\n  "OPENAI_API_KEY": "sk-test"\n}')
    expect(wrapper.text()).not.toContain('auth.json')

    const restartNotice = wrapper.get('[data-testid="codex-api-key-restart-notice"]')
    expect(restartNotice.text()).toContain(
      'keys.useKeyModal.openai.authModeApiKeyRestartNotice'
    )

    await wrapper.get('[data-testid="codex-auth-mode-legacy"]').trigger('click')
    await nextTick()

    expect(wrapper.find('[data-testid="codex-api-key-restart-notice"]').exists()).toBe(false)
    expect(wrapper.findAll('pre code').map((code) => code.text()).join('\n')).not.toContain(
      'x-openai-actor-authorization'
    )
  })

  it('keeps legacy OpenAI Codex WebSocket config as the default', async () => {
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

    const wsTab = wrapper.findAll('button').find((button) =>
      button.text().includes('keys.useKeyModal.cliTabs.codexCliWs')
    )

    expect(wsTab).toBeDefined()
    await wsTab!.trigger('click')
    await nextTick()

    const codeBlocks = wrapper.findAll('pre code').map((code) => code.text())
    const configToml = codeBlocks.find((content) => content.includes('supports_websockets = true'))

    expect(configToml).toBeDefined()
    expect(configToml).toContain('model = "gpt-5.5"')
    expect(configToml).toContain('review_model = "gpt-5.5"')
    expect(configToml).not.toContain('model = "gpt-5.4"')
    expect(configToml).not.toContain('model_context_window')
    expect(configToml).not.toContain('model_auto_compact_token_limit')
    expect(configToml).toContain('requires_openai_auth = true')
    expect(configToml).not.toContain('experimental_bearer_token')
    expect(configToml).not.toContain('x-openai-actor-authorization')
    expect(configToml).not.toContain('env_key')
    expect(configToml).not.toContain('image_generation')
    expect(configToml).toContain('supports_websockets = true')
    expect(configToml).toContain('[features]\nresponses_websockets_v2 = true\ngoals = true')
    expect(codeBlocks).toContain('{\n  "OPENAI_API_KEY": "sk-test"\n}')
    expect(wrapper.text()).toContain('auth.json')
  })

  it('preserves API Key Mode when switching to OpenAI Codex WebSocket config', async () => {
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

    const apiKeyMode = wrapper.get('[data-testid="codex-auth-mode-api-key"]')
    await apiKeyMode.trigger('click')

    const wsTab = wrapper.findAll('button').find((button) =>
      button.text().includes('keys.useKeyModal.cliTabs.codexCliWs')
    )
    expect(wsTab).toBeDefined()
    await wsTab!.trigger('click')
    await nextTick()

    const codeBlocks = wrapper.findAll('pre code').map((code) => code.text())
    const configToml = codeBlocks.find((content) => content.includes('supports_websockets = true'))
    expect(configToml).toContain('model_catalog_url = "https://example.com/v1/models"')
    expect(configToml).not.toContain('model_catalog_json')

    expect(wrapper.get('[data-testid="codex-auth-mode-api-key"]').attributes('aria-checked')).toBe('true')
    expect(configToml).toBeDefined()
    expect(configToml).toContain('requires_openai_auth = false')
    expect(configToml).toContain('experimental_bearer_token = "sk-test"')
    expect(configToml).toContain('http_headers = { "x-openai-actor-authorization" = "local-image-extension" }')
    expect(configToml).not.toContain('env_key')
    expect(configToml).not.toContain('image_generation')
    expect(configToml).toContain('supports_websockets = true')
    expect(configToml).toContain('[features]\nresponses_websockets_v2 = true\ngoals = true')
    expect(codeBlocks).not.toContain('{\n  "OPENAI_API_KEY": "sk-test"\n}')
    expect(wrapper.text()).not.toContain('auth.json')
  })

  it('resets Codex authentication mode when the modal reopens or platform changes', async () => {
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

    await wrapper.get('[data-testid="codex-auth-mode-api-key"]').trigger('click')
    await wrapper.setProps({ show: false })
    await wrapper.setProps({ show: true })
    await nextTick()

    expect(wrapper.get('[data-testid="codex-auth-mode-legacy"]').attributes('aria-checked')).toBe('true')
    expect(wrapper.findAll('pre code').map((code) => code.text()).join('\n')).toContain('requires_openai_auth = true')

    await wrapper.get('[data-testid="codex-auth-mode-api-key"]').trigger('click')
    await wrapper.setProps({ platform: 'gemini' })
    await wrapper.setProps({ platform: 'openai' })
    await nextTick()

    expect(wrapper.get('[data-testid="codex-auth-mode-legacy"]').attributes('aria-checked')).toBe('true')
    expect(wrapper.findAll('pre code').map((code) => code.text()).join('\n')).not.toContain('x-openai-actor-authorization')
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

  it('renders GPT-5.6 and GPT-6 Astra capabilities in OpenCode config', async () => {
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

    const parsed = JSON.parse(wrapper.find('pre code').text())
    const models = parsed.provider.openai.models
    for (const model of ['gpt-5.6', 'gpt-5.6-sol', 'gpt-5.6-terra', 'gpt-5.6-luna', 'gpt-6.1-sol', 'gpt-6-sol', 'gpt-6-luna']) {
      expect(models[model]).toBeDefined()
      expect(models[model].variants).toHaveProperty('max')
      expect(models[model].variants).toHaveProperty('xhigh')
    }
    expect(models['gpt-5.6'].name).toBe('GPT-5.6 (Sol)')
    expect(models['gpt-6.1-sol'].variants).not.toHaveProperty('none')
    expect(models['gpt-6-sol'].variants).toHaveProperty('none')
    expect(models['gpt-6-luna'].limit).toEqual({ context: 1050000, output: 128000 })
    expect(models['gpt-6']).toEqual({
      name: 'GPT-6 (Astra)',
      limit: { context: 1050000, output: 128000 },
      options: { store: false },
      variants: { low: {}, medium: {}, high: {}, xhigh: {}, max: {} }
    })
    expect(models['gpt-6-astra']).toEqual({
      name: 'GPT-6 Astra',
      limit: { context: 1050000, output: 128000 },
      options: { store: false },
      variants: { low: {}, medium: {}, high: {}, xhigh: {}, max: {} }
    })
  })

  it('exports Claude 5.5 models on the Anthropic provider with adaptive defaults', async () => {
    const wrapper = mount(UseKeyModal, {
      props: { show: true, apiKey: 'sk-test', baseUrl: 'https://example.com/v1', platform: 'anthropic' },
      global: { stubs: { BaseDialog: { template: '<div><slot /><slot name="footer" /></div>' }, Icon: { template: '<span />' } } }
    })
    const tab = wrapper.findAll('button').find(button => button.text().includes('keys.useKeyModal.cliTabs.opencode'))
    expect(tab).toBeDefined()
    await tab!.trigger('click')
    await nextTick()
    const models = JSON.parse(wrapper.find('pre code').text()).provider.anthropic.models
    expect(models['claude-opus-5-5'].limit).toEqual({ context: 1000000, output: 128000 })
    expect(models['claude-opus-5-5'].options).toEqual({ thinking: { type: 'adaptive' }, effort: 'medium' })
    expect(models['claude-opus-5-5'].variants.xhigh.effort).toBe('xhigh')
    expect(models['claude-opus-5-5'].variants).not.toHaveProperty('none')
    expect(models['claude-sonnet-5-5'].limit).toEqual({ context: 1000000, output: 128000 })
    expect(models['claude-sonnet-5-5'].options).toEqual({ thinking: { type: 'adaptive' }, effort: 'high' })
    expect(models['claude-sonnet-5-5'].variants.xhigh.effort).toBe('xhigh')
    expect(models['claude-sonnet-5-5'].variants).not.toHaveProperty('none')
  })

  it('renders Claude Fable 5 OpenCode config with adaptive thinking', async () => {
    const wrapper = mount(UseKeyModal, {
      props: {
        show: true,
        apiKey: 'sk-test',
        baseUrl: 'https://example.com/v1',
        platform: 'antigravity'
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

    const claudeConfig = wrapper.findAll('pre code')
      .map((code) => code.text())
      .find((content) => content.includes('"antigravity-claude"'))

    expect(claudeConfig).toBeDefined()
    const parsed = JSON.parse(claudeConfig!)
    const fable51 = parsed.provider['antigravity-claude'].models['claude-fable-5-1']
    const fable = parsed.provider['antigravity-claude'].models['claude-fable-5']

    expect(fable51.name).toBe('Claude Fable 5.1')
    expect(fable51.limit).toEqual({ context: 1048576, output: 128000 })
    expect(fable51.options.thinking).toEqual({ type: 'adaptive' })
    expect(fable51.options.thinking).not.toHaveProperty('budgetTokens')
    expect(fable.name).toBe('Claude Fable 5')
    expect(fable.limit).toEqual({ context: 1048576, output: 128000 })
    expect(fable.options.thinking).toEqual({ type: 'adaptive' })
    expect(fable.options.thinking).not.toHaveProperty('budgetTokens')
  })

  // Scenario: API Key users can load a routed catalog remotely or download it for older clients.
  it('offers remote and downloadable Codex catalogs for Composite API keys', async () => {
    const manifest = {
      models: [
        {
          slug: 'claude-opus-4-8',
          default_reasoning_level: 'medium',
          supported_reasoning_levels: [{ effort: 'max', description: 'Maximum reasoning depth' }],
          input_modalities: ['text'],
          model_messages: { instructions_template: 'Use the routed model.' }
        },
        {
          slug: 'grok-4.6',
          default_reasoning_level: 'high',
          supported_reasoning_levels: [{ effort: 'xhigh', description: 'Extra-high reasoning depth' }],
          input_modalities: ['text'],
          model_messages: { instructions_template: 'Use the routed model.' }
        }
      ]
    }
    const fetchMock = vi.fn().mockResolvedValue({
      ok: true,
      status: 200,
      text: async () => JSON.stringify(manifest)
    })
    vi.stubGlobal('fetch', fetchMock)

    const wrapper = mount(UseKeyModal, {
      props: {
        show: true,
        apiKey: 'sk-composite-test',
        baseUrl: 'https://example.com/v1',
        platform: 'composite'
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

    const codexTab = wrapper.findAll('button').find((button) =>
      button.text().includes('keys.useKeyModal.cliTabs.codexCli')
    )
    expect(codexTab).toBeDefined()
    await codexTab!.trigger('click')
    await nextTick()

    const unixConfig = wrapper.findAll('pre code')
      .map((code) => code.text())
      .find((content) => content.includes('[model_providers.sub2api]'))
    expect(unixConfig).toContain('[model_providers.sub2api]\nname = "Sub2API Composite"\nbase_url = "https://example.com/v1"\nmodel_catalog_url = "https://example.com/v1/models"')
    expect(unixConfig).not.toContain('model_catalog_json')
    expect(unixConfig).toContain('env_key = "SUB2API_API_KEY"')
    expect(fetchMock).not.toHaveBeenCalled()

    await wrapper.get('[data-testid="codex-model-catalog-mode"]').setValue('file')

    await wrapper.get('[data-testid="codex-model-catalog-fetch"]').trigger('click')
    await flushPromises()

    expect(fetchMock).toHaveBeenCalledWith(
      'https://example.com/v1/models?client_version=0.158.0',
      expect.objectContaining({
        headers: expect.objectContaining({ Authorization: 'Bearer sk-composite-test' })
      })
    )
    expect(wrapper.get('[data-testid="codex-model-catalog"]').text())
      .toContain('keys.useKeyModal.codexModelCatalog.download')

    const loadedUnixConfig = wrapper.findAll('pre code')
      .map((code) => code.text())
      .find((content) => content.includes('[model_providers.sub2api]'))
    expect(loadedUnixConfig).toContain('model = "claude-opus-4-8"')
    expect(loadedUnixConfig).toContain('review_model = "claude-opus-4-8"')
    expect(loadedUnixConfig).not.toContain('model = "gpt-5.5"')
    expect(loadedUnixConfig).toContain('model_catalog_json = "~/.codex/codex-models.json"')
    expect(loadedUnixConfig).not.toContain('model_catalog_url')

    const downloadButton = wrapper.findAll('button').find((button) =>
      button.text().includes('keys.useKeyModal.codexModelCatalog.download')
    )
    expect(downloadButton).toBeDefined()
    await downloadButton!.trigger('click')
    expect(saveAsMock).toHaveBeenCalledWith(expect.any(Blob), 'codex-models.json')
    const downloadedBlob = saveAsMock.mock.calls[0]?.[0] as Blob
    expect(JSON.parse(await readBlobAsText(downloadedBlob))).toEqual(manifest)

    const windowsTab = wrapper.findAll('button').find((button) => button.text().trim() === 'Windows')
    expect(windowsTab).toBeDefined()
    await windowsTab!.trigger('click')
    await nextTick()

    const windowsConfig = wrapper.findAll('pre code')
      .map((code) => code.text())
      .find((content) => content.includes('[model_providers.sub2api]'))
    // Codex does not expand %userprofile% in config.toml; it only expands ~/.
    expect(windowsConfig).toContain('model_catalog_json = "~/.codex/codex-models.json"')
    expect(windowsConfig).not.toContain('%userprofile%')
    expect(wrapper.get('[data-testid="codex-model-catalog"]').text())
      .toContain('%userprofile%\\.codex\\codex-models.json')
  })

  it.each(['anthropic', 'gemini', 'antigravity', 'grok', 'kimi', 'zhipu', 'deepseek', 'minimax', 'opencode_go'] as const)(
    'offers Codex catalog configuration for the %s routed group',
    async (platform) => {
      const wrapper = mount(UseKeyModal, {
        props: {
          show: true,
          apiKey: `sk-${platform}-test`,
          baseUrl: 'https://example.com/v1',
          platform
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

      const codexTab = wrapper.findAll('button').find((button) =>
        button.text().includes('keys.useKeyModal.cliTabs.codexCli')
      )
      expect(codexTab).toBeDefined()
      await codexTab!.trigger('click')
      await nextTick()

      expect(wrapper.find('[data-testid="codex-model-catalog"]').exists()).toBe(true)
      const config = wrapper.findAll('pre code')
        .map((code) => code.text())
        .find((content) => content.includes('[model_providers.sub2api]'))
      expect(config).toContain('model_catalog_url = "https://example.com/v1/models"')
      expect(config).not.toContain('model_catalog_json')
      expect(config).toContain('base_url = "https://example.com/v1"')
      expect(config).toContain('wire_api = "responses"')
    }
  )

  // Scenario: the platform-preferred model remains selected when the downloaded catalog contains it.
  it('keeps the preferred Composite default when it exists in the catalog', async () => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue({
      ok: true,
      status: 200,
      text: async () => JSON.stringify({
        models: [
          { slug: 'claude-opus-4-8' },
          { slug: 'gpt-5.5' }
        ]
      })
    }))

    const wrapper = mount(UseKeyModal, {
      props: {
        show: true,
        apiKey: 'sk-composite-test',
        baseUrl: 'https://example.com/v1',
        platform: 'composite'
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

    const codexTab = wrapper.findAll('button').find((button) =>
      button.text().includes('keys.useKeyModal.cliTabs.codexCli')
    )
    expect(codexTab).toBeDefined()
    await codexTab!.trigger('click')
    await wrapper.get('[data-testid="codex-model-catalog-fetch"]').trigger('click')
    await flushPromises()

    const config = wrapper.findAll('pre code')
      .map((code) => code.text())
      .find((content) => content.includes('[model_providers.sub2api]'))
    expect(config).toContain('model = "gpt-5.5"')
    expect(config).toContain('review_model = "gpt-5.5"')
  })

  it('offers remote and optional file catalogs for OpenAI in both transport modes and on both platforms', async () => {
    const fetchMock = vi.fn()
    vi.stubGlobal('fetch', fetchMock)
    const wrapper = mount(UseKeyModal, {
      props: {
        show: true,
        apiKey: 'sk-openai-test',
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

    for (const transport of ['keys.useKeyModal.cliTabs.codexCli', 'keys.useKeyModal.cliTabs.codexCliWs']) {
      await wrapper.findAll('button').find((button) => button.text().trim() === transport)!.trigger('click')
      for (const os of ['macOS / Linux', 'Windows']) {
        await wrapper.findAll('button').find((button) => button.text().trim() === os)!.trigger('click')
        const configToml = wrapper.findAll('pre code')
          .map((code) => code.text())
          .find((content) => content.includes('model_provider = "OpenAI"'))
        expect(configToml).toContain('model = "gpt-5.5"')
        expect(configToml).toContain('[model_providers.OpenAI]\nname = "OpenAI"\nbase_url = "https://example.com/v1"\nmodel_catalog_url = "https://example.com/v1/models"')
        expect(configToml).not.toContain('model_catalog_json')
        expect(configToml).toContain('requires_openai_auth = true')
        expect(configToml).not.toContain('client_version')
        expect(wrapper.find('[data-testid="codex-model-catalog"]').exists()).toBe(true)

        await wrapper.get('[data-testid="codex-model-catalog-mode"]').setValue('file')
        const fileConfig = wrapper.findAll('pre code')
          .map((code) => code.text())
          .find((content) => content.includes('model_provider = "OpenAI"'))!
        expect(fileConfig).toContain('model_catalog_json = "~/.codex/codex-models.json"')
        expect(fileConfig).not.toContain('model_catalog_url')
        expect(fileConfig.indexOf('model_catalog_json')).toBeLessThan(fileConfig.indexOf('[model_providers.OpenAI]'))
        await wrapper.get('[data-testid="codex-model-catalog-mode"]').setValue('remote')
      }
    }
    expect(fetchMock).not.toHaveBeenCalled()
  })

  it.each([1024 * 1024 - 1, 1024 * 1024, 1024 * 1024 + 1])(
    'uses file mode only when the manifest exceeds the remote byte limit (%s bytes)',
    async (responseBytes) => {
      const manifest = { models: [{ slug: 'gpt-5.5', description: '' }] }
      manifest.models[0]!.description = 'x'.repeat(responseBytes - JSON.stringify(manifest).length)
      const responseText = JSON.stringify(manifest)
      vi.stubGlobal('fetch', vi.fn().mockResolvedValue({
        ok: true,
        status: 200,
        text: async () => responseText
      }))
      const wrapper = mount(UseKeyModal, {
        props: {
          show: true,
          apiKey: 'sk-test',
          baseUrl: 'https://example.com/v1',
          platform: 'openai'
        },
        global: {
          stubs: {
            BaseDialog: { template: '<div><slot /><slot name="footer" /></div>' },
            Icon: { template: '<span />' }
          }
        }
      })

      await wrapper.get('[data-testid="codex-model-catalog-fetch"]').trigger('click')
      await flushPromises()

      const oversized = responseBytes > 1024 * 1024
      const mode = wrapper.get('[data-testid="codex-model-catalog-mode"]')
      expect((mode.element as HTMLSelectElement).value).toBe(oversized ? 'file' : 'remote')
      expect((mode.get('option[value="remote"]').element as HTMLOptionElement).disabled).toBe(oversized)
      const config = wrapper.findAll('pre code').map((code) => code.text())
        .find((content) => content.includes('model_provider = "OpenAI"'))!
      if (oversized) {
        expect(config).toContain('model_catalog_json = "~/.codex/codex-models.json"')
        expect(config).not.toContain('model_catalog_url')
        expect(wrapper.text()).toContain('keys.useKeyModal.codexModelCatalog.oversized')
        await wrapper.findAll('button').find((button) =>
          button.text().includes('keys.useKeyModal.codexModelCatalog.download')
        )!.trigger('click')
        const downloaded = saveAsMock.mock.calls[0]?.[0] as Blob
        expect(JSON.parse(await readBlobAsText(downloaded))).toEqual(manifest)

        await wrapper.setProps({ apiKey: 'sk-new-group' })
        expect((mode.get('option[value="remote"]').element as HTMLOptionElement).disabled).toBe(false)
        expect(wrapper.text()).not.toContain('keys.useKeyModal.codexModelCatalog.oversized')
      } else {
        expect(config).toContain('model_catalog_url = "https://example.com/v1/models"')
        expect(config).not.toContain('model_catalog_json')
      }
    }
  )
})
