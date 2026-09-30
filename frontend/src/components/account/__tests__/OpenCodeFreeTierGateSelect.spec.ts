import { mount } from '@vue/test-utils'
import { describe, expect, it, vi } from 'vitest'
import OpenCodeFreeTierGateSelect from '../OpenCodeFreeTierGateSelect.vue'
import {
  DEFAULT_OPENCODE_FREE_TIER_GATE,
  OPENCODE_FREE_TIER_GATE_KEY,
  applyOpenCodeFreeTierGate,
  resolveOpenCodeFreeTierGate
} from '../credentialsBuilder'

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return {
    ...actual,
    useI18n: () => ({
      t: (key: string) => key
    })
  }
})

const selectOf = (wrapper: ReturnType<typeof mount>) =>
  wrapper.get('[data-testid="opencode-free-tier-gate-select"]')

describe('OpenCodeFreeTierGateSelect', () => {
  it('renders the three gate modes', () => {
    const wrapper = mount(OpenCodeFreeTierGateSelect, { props: { mode: 'auto' } })
    const options = selectOf(wrapper).findAll('option').map(option => option.element.value)

    expect(options).toEqual(['auto', 'always', 'off'])
  })

  it('shows the hint matching the current mode', () => {
    const wrapper = mount(OpenCodeFreeTierGateSelect, { props: { mode: 'off' } })

    expect(wrapper.text()).toContain('admin.accounts.opencodeGo.freeTierGate.offHint')
  })

  it('emits update:mode when the selection changes', async () => {
    const wrapper = mount(OpenCodeFreeTierGateSelect, { props: { mode: 'auto' } })

    await selectOf(wrapper).setValue('always')

    expect(wrapper.emitted('update:mode')).toEqual([['always']])
  })

  it('falls back to auto for an unknown mode', () => {
    const wrapper = mount(OpenCodeFreeTierGateSelect, {
      props: { mode: 'nonsense' as never }
    })

    expect((selectOf(wrapper).element as HTMLSelectElement).value).toBe('auto')
  })
})

describe('resolveOpenCodeFreeTierGate', () => {
  it('accepts the three supported modes', () => {
    expect(resolveOpenCodeFreeTierGate('auto')).toBe('auto')
    expect(resolveOpenCodeFreeTierGate('always')).toBe('always')
    expect(resolveOpenCodeFreeTierGate('off')).toBe('off')
  })

  it('falls back to auto for missing or invalid values', () => {
    expect(resolveOpenCodeFreeTierGate(undefined)).toBe('auto')
    expect(resolveOpenCodeFreeTierGate(null)).toBe('auto')
    expect(resolveOpenCodeFreeTierGate('sometimes')).toBe('auto')
    expect(resolveOpenCodeFreeTierGate(1)).toBe('auto')
    expect(resolveOpenCodeFreeTierGate(true)).toBe('auto')
  })
})

describe('applyOpenCodeFreeTierGate', () => {
  it('omits the default on create so the stored credentials stay minimal', () => {
    const credentials: Record<string, unknown> = {}

    applyOpenCodeFreeTierGate(credentials, DEFAULT_OPENCODE_FREE_TIER_GATE, 'create')

    expect(credentials).not.toHaveProperty(OPENCODE_FREE_TIER_GATE_KEY)
  })

  it('writes a non-default mode on create', () => {
    const credentials: Record<string, unknown> = {}

    applyOpenCodeFreeTierGate(credentials, 'off', 'create')

    expect(credentials[OPENCODE_FREE_TIER_GATE_KEY]).toBe('off')
  })

  it('always writes on edit so a non-default mode can be reverted to auto', () => {
    const credentials: Record<string, unknown> = { [OPENCODE_FREE_TIER_GATE_KEY]: 'off' }

    applyOpenCodeFreeTierGate(credentials, 'auto', 'edit')

    expect(credentials[OPENCODE_FREE_TIER_GATE_KEY]).toBe('auto')
  })

  it('round-trips through resolve', () => {
    const credentials: Record<string, unknown> = {}
    applyOpenCodeFreeTierGate(credentials, 'always', 'edit')

    expect(resolveOpenCodeFreeTierGate(credentials[OPENCODE_FREE_TIER_GATE_KEY])).toBe('always')
  })
})
