import { describe, expect, it, vi } from 'vitest'
import { mount, type VueWrapper } from '@vue/test-utils'
import { nextTick } from 'vue'
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

// The common Select teleports its dropdown to body; the data-testid lands on
// the Select root div, so open the panel of that container and pick an option
// by its translated label key.
async function pickProtocolOption(wrapper: VueWrapper, index: number, label: string) {
  // Close any lingering open panel first (leave transition keeps closed panels
  // in the DOM for two rAF frames in jsdom).
  for (const trigger of wrapper.findAll('button[aria-haspopup="true"]')) {
    if (trigger.attributes('aria-expanded') === 'true') await trigger.trigger('click')
  }
  await nextTick()
  await new Promise(resolve => setTimeout(resolve, 50))
  const container = wrapper.get(`[data-testid="opencode-go-protocol-select-${index}"]`)
  await container.find('button[aria-haspopup="true"]').trigger('click')
  await nextTick()
  const portals = document.body.querySelectorAll<HTMLElement>('.select-dropdown-portal')
  const portal = [...portals].find((el) => el.style.display !== 'none' && el.querySelector('.select-option'))
  expect(portal, `expected an open dropdown portal for select ${index}`).toBeTruthy()
  const option = [...portal!.querySelectorAll('.select-option')]
    .find((el) => el.textContent?.trim() === label && !el.className.includes('select-option-group'))
  expect(option, `option "${label}" not found`).toBeDefined()
  ;(option as HTMLElement).click()
  await nextTick()
  await new Promise(resolve => setTimeout(resolve, 50))
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

    await pickProtocolOption(wrapper, 0, 'admin.accounts.cnProviders.apiProtocol.chatCompletions')
    expect(rows[0]).toEqual({ pattern: 'gpt-*', protocol: 'chat_completions', extraProtocols: ['responses'] })

    await pickProtocolOption(wrapper, 0, 'admin.accounts.cnProviders.apiProtocol.anthropic')
    expect(rows[0]).toEqual({ pattern: 'gpt-*', protocol: 'anthropic', extraProtocols: ['responses'] })

    await pickProtocolOption(wrapper, 1, 'admin.accounts.cnProviders.apiProtocol.responses')
    expect(rows[1]).toEqual({ pattern: 'grok-*', protocol: 'responses' })
  })

  it('describes the model-list fallback only for providers with a model catalog', () => {
    expect(mountEditor([]).get('[data-testid="opencode-go-protocol-fallback"]').text())
      .toContain('admin.accounts.opencodeGo.protocolRules.catalogFallback')
    expect(mountEditor([], 'opencode_go', 'go').get('[data-testid="opencode-go-protocol-fallback"]').text())
      .toContain('admin.accounts.opencodeGo.protocolRules.fallback')
  })
})
