import { describe, expect, it, vi } from 'vitest'
import { mount } from '@vue/test-utils'
import { nextTick } from 'vue'
import { readFileSync } from 'node:fs'
import { resolve } from 'node:path'

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
