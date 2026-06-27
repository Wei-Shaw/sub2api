import { describe, expect, it, vi } from 'vitest'
import { mount } from '@vue/test-utils'
import { nextTick } from 'vue'

vi.mock('vue-i18n', () => ({
  useI18n: () => ({
    t: (key: string) => key,
    locale: { value: 'en' },
  }),
}))

import PlatformProfileRegistryEditor from '../PlatformProfileRegistryEditor.vue'
import {
  appendPlatformProfileTemplate,
  formatPlatformProfileRegistryInput,
  validatePlatformProfileRegistryInput,
} from '../platformProfileRegistry'

describe('platformProfileRegistry helpers', () => {
  it('treats an empty registry as valid DB-backed default state', () => {
    const validation = validatePlatformProfileRegistryInput('')

    expect(validation.ok).toBe(true)
    expect(validation.isEmpty).toBe(true)
    expect(validation.registry).toBeNull()
    expect(validation.warnings.join('\n')).toContain('OpenAI, Anthropic, Gemini')
  })

  it('formats and appends platform templates in a stable way', () => {
    const formatted = formatPlatformProfileRegistryInput('{"version":1,"profiles":[{"platform":"openai","guide":{"title":"OpenAI","description":"Guide","clients":[{"id":"codex","label":"Codex"}],"copy_blocks":[{"id":"codex-config","client_id":"codex","path":"Terminal","content_template":"echo 1"}]}}]}')
    expect(formatted).toContain('"platform": "openai"')
    expect(formatted).toContain('  "profiles": [')

    const appended = appendPlatformProfileTemplate('')
    expect(appended).not.toBeNull()
    expect(appended?.platform).toBe('new-platform')
    expect(appended?.value).toContain('"platform": "new-platform"')
    expect(appended?.value).toContain('new-platform delivery guide')
  })
})

describe('PlatformProfileRegistryEditor', () => {
  it('shows a live preview and emits inserted template JSON', async () => {
    const registry = JSON.stringify({
      version: 1,
      profiles: [
        {
          platform: 'openai',
          provider_id: 'v-claw-openai',
          provider_name: 'OpenAI',
          api_style: 'openai-responses',
          guide: {
            profile_id: 'openai',
            title: 'OpenAI guide',
            description: 'Use OpenAI-compatible clients',
            note: 'DB-backed note',
            docs_url: 'https://example.com/docs',
            default_client: 'codex',
            clients: [{ id: 'codex', label: 'Codex CLI', os: ['unix'] }],
            copy_blocks: [
              {
                id: 'openai-codex-config',
                client_id: 'codex',
                os: 'unix',
                path: '~/.codex/config.toml',
                hint: 'Codex config',
                language: 'toml',
                content_template: 'model = "gpt-5.5"',
              },
            ],
          },
        },
      ],
    })

    const wrapper = mount(PlatformProfileRegistryEditor, {
      props: {
        modelValue: registry,
      },
      global: {
        stubs: {
          Icon: {
            template: '<span />',
          },
        },
      },
    })

    expect(wrapper.text()).toContain('Schema hợp lệ')
    expect(wrapper.text()).toContain('OpenAI guide')
    expect(wrapper.text()).toContain('OpenAI-compatible clients')
    expect(wrapper.text()).toContain('DB-backed note')
    expect(wrapper.text()).toContain('Platforms')
    expect(wrapper.text()).toContain('Clients')
    expect(wrapper.text()).toContain('Blocks')

    await wrapper.findAll('button').find((button) =>
      button.text().includes('Insert platform mẫu'),
    )!.trigger('click')
    await nextTick()

    const emitted = wrapper.emitted('update:modelValue')
    expect(emitted).toBeTruthy()
    expect(emitted?.at(-1)?.[0]).toContain('"platform": "new-platform"')
    expect(emitted?.at(-1)?.[0]).toContain('new-platform delivery guide')
  })
})
