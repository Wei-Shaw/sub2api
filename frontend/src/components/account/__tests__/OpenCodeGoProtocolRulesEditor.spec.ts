import { describe, expect, it, vi } from 'vitest'
import { mount } from '@vue/test-utils'
import OpenCodeGoProtocolRulesEditor from '../OpenCodeGoProtocolRulesEditor.vue'
import type { OpenCodeGoProtocolRule } from '../credentialsBuilder'

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return {
    ...actual,
    useI18n: () => ({ t: (key: string) => key })
  }
})

function mountEditor(rows: OpenCodeGoProtocolRule[], platform = 'command_code', plan = 'payg') {
  return mount(OpenCodeGoProtocolRulesEditor, {
    props: { rows, platform, plan },
    global: { stubs: { Icon: true } }
  })
}

describe('OpenCodeGoProtocolRulesEditor protocol sets', () => {
  it('toggles additional protocols in protocol order', async () => {
    const rows: OpenCodeGoProtocolRule[] = [{ pattern: 'gpt-*', protocol: 'anthropic' }]
    const wrapper = mountEditor(rows)

    expect(wrapper.find('[data-testid="opencode-go-protocol-extra-0-anthropic"]').exists()).toBe(false)
    const responses = wrapper.get('[data-testid="opencode-go-protocol-extra-0-responses"]')
    expect(responses.attributes('aria-pressed')).toBe('false')

    await responses.trigger('click')
    await wrapper.get('[data-testid="opencode-go-protocol-extra-0-chat_completions"]').trigger('click')
    expect(rows[0].extraProtocols).toEqual(['chat_completions', 'responses'])
    expect(wrapper.get('[data-testid="opencode-go-protocol-extra-0-responses"]').attributes('aria-pressed')).toBe('true')

    await wrapper.get('[data-testid="opencode-go-protocol-extra-0-responses"]').trigger('click')
    await wrapper.get('[data-testid="opencode-go-protocol-extra-0-chat_completions"]').trigger('click')
    expect(rows[0]).toEqual({ pattern: 'gpt-*', protocol: 'anthropic' })
  })

  it('swaps the preferred protocol within the set and replaces it otherwise', async () => {
    const rows: OpenCodeGoProtocolRule[] = [
      { pattern: 'gpt-*', protocol: 'responses', extraProtocols: ['chat_completions'] },
      { pattern: 'grok-*', protocol: 'chat_completions' }
    ]
    const wrapper = mountEditor(rows)

    await wrapper.get('[data-testid="opencode-go-protocol-select-0"]').setValue('chat_completions')
    expect(rows[0]).toEqual({ pattern: 'gpt-*', protocol: 'chat_completions', extraProtocols: ['responses'] })

    await wrapper.get('[data-testid="opencode-go-protocol-select-0"]').setValue('anthropic')
    expect(rows[0]).toEqual({ pattern: 'gpt-*', protocol: 'anthropic', extraProtocols: ['responses'] })

    await wrapper.get('[data-testid="opencode-go-protocol-select-1"]').setValue('responses')
    expect(rows[1]).toEqual({ pattern: 'grok-*', protocol: 'responses' })
  })

  it('describes the model-list fallback only for providers with a model catalog', () => {
    expect(mountEditor([]).get('[data-testid="opencode-go-protocol-fallback"]').text())
      .toContain('admin.accounts.opencodeGo.protocolRules.catalogFallback')
    expect(mountEditor([], 'opencode_go', 'go').get('[data-testid="opencode-go-protocol-fallback"]').text())
      .toContain('admin.accounts.opencodeGo.protocolRules.fallback')
  })
})
