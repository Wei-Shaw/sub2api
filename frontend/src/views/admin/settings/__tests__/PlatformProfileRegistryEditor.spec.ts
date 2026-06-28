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

function registryFixture() {
  return JSON.stringify({
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
              content_template: 'model = "{{openai_model}}"',
            },
          ],
        },
      },
    ],
  })
}

function mountEditor(modelValue = registryFixture()) {
  return mount(PlatformProfileRegistryEditor, {
    props: {
      modelValue,
    },
    global: {
      stubs: {
        Icon: {
          template: '<span />',
        },
      },
    },
  })
}

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
  it('shows a visual platform workspace and emits inserted template JSON', async () => {
    const wrapper = mountEditor()

    expect(wrapper.text()).toContain('Schema hợp lệ')
    expect(wrapper.text()).toContain('OpenAI guide')
    expect(wrapper.text()).toContain('Use OpenAI-compatible clients')
    expect(wrapper.text()).toContain('DB-backed note')
    expect(wrapper.text()).toContain('Platforms')
    expect(wrapper.text()).toContain('Clients')
    expect(wrapper.text()).toContain('Blocks')
    expect(wrapper.find('[data-test="registry-visual-editor"]').exists()).toBe(true)
    expect(wrapper.find('[data-test="raw-json-textarea"]').exists()).toBe(false)

    await wrapper.find('[data-test="add-platform-button"]').trigger('click')
    await nextTick()

    const emitted = wrapper.emitted('update:modelValue')
    expect(emitted).toBeTruthy()
    expect(emitted?.at(-1)?.[0]).toContain('"platform": "new-platform"')
    expect(emitted?.at(-1)?.[0]).toContain('new-platform delivery guide')
  })

  it('emits normalized JSON when operator edits guide title through the visual form', async () => {
    const wrapper = mountEditor()

    await wrapper.find('[data-test="guide-title-input"]').setValue('Updated OpenAI guide')
    await nextTick()

    const emitted = wrapper.emitted('update:modelValue')
    const payload = JSON.parse(emitted?.at(-1)?.[0] as string)
    expect(payload.profiles[0].guide.title).toBe('Updated OpenAI guide')
    expect(payload.profiles[0].platform).toBe('openai')
  })

  it('keeps the visual editor mounted while a required text field is temporarily invalid', async () => {
    const wrapper = mountEditor()

    await wrapper.find('[data-test="guide-title-input"]').setValue('')
    await nextTick()
    await wrapper.setProps({
      modelValue: wrapper.emitted('update:modelValue')?.at(-1)?.[0] as string,
    })

    expect(wrapper.text()).toContain('Schema lỗi')
    expect(wrapper.text()).toContain('profiles[0].guide.title là bắt buộc')
    expect(wrapper.find('[data-test="registry-visual-editor"]').exists()).toBe(true)
    expect(wrapper.find('[data-test="guide-title-input"]').exists()).toBe(true)
  })

  it('adds clients and copy blocks without editing escaped raw JSON', async () => {
    const wrapper = mountEditor()

    await wrapper.find('[data-test="add-client-button"]').trigger('click')
    await nextTick()
    await wrapper.setProps({
      modelValue: wrapper.emitted('update:modelValue')?.at(-1)?.[0] as string,
    })

    expect(wrapper.findAll('[data-test="client-row"]')).toHaveLength(2)

    await wrapper.find('[data-test="add-copy-block-button"]').trigger('click')
    await nextTick()
    await wrapper.setProps({
      modelValue: wrapper.emitted('update:modelValue')?.at(-1)?.[0] as string,
    })

    expect(wrapper.findAll('[data-test="copy-block-tab"]')).toHaveLength(2)

    await wrapper.find('[data-test="placeholder-button"]').trigger('click')
    await nextTick()

    const payload = JSON.parse(wrapper.emitted('update:modelValue')?.at(-1)?.[0] as string)
    expect(payload.profiles[0].guide.clients).toHaveLength(2)
    expect(payload.profiles[0].guide.copy_blocks).toHaveLength(2)
    expect(payload.profiles[0].guide.copy_blocks[1].content_template).toContain('{{base_url}}')
  })

  it('keeps raw JSON available behind advanced mode', async () => {
    const wrapper = mountEditor()

    expect(wrapper.find('[data-test="raw-json-textarea"]').exists()).toBe(false)
    await wrapper.find('[data-test="advanced-json-toggle"]').trigger('click')
    await nextTick()

    const rawEditor = wrapper.find('[data-test="raw-json-textarea"]')
    expect(rawEditor.exists()).toBe(true)
    await rawEditor.setValue('{"version":1,"profiles":[]}')

    expect(wrapper.emitted('update:modelValue')?.at(-1)?.[0]).toBe('{"version":1,"profiles":[]}')
  })
})
