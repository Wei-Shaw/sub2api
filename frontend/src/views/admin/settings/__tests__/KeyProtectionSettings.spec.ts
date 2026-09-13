import { flushPromises, mount } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import KeyProtectionSettings from '../KeyProtectionSettings.vue'

const { getConfig, updateConfig, clearMappings, getGroups, showSuccess } = vi.hoisted(() => ({
  getConfig: vi.fn(), updateConfig: vi.fn(), clearMappings: vi.fn(), getGroups: vi.fn(), showSuccess: vi.fn(),
}))

vi.mock('vue-i18n', () => ({ useI18n: () => ({ t: (key: string) => key }) }))
vi.mock('@/api/admin/keyProtection', () => ({
  getKeyProtectionConfig: getConfig,
  updateKeyProtectionConfig: updateConfig,
  clearKeyProtectionMappings: clearMappings,
}))
vi.mock('@/api/admin/groups', () => ({ getAllIncludingInactive: getGroups }))
vi.mock('@/api/admin', () => ({ adminAPI: {} }))
vi.mock('@/stores', () => ({ useAppStore: () => ({ showSuccess }) }))

const defaults = {
  enabled: false, user_ids: [], group_ids: [], mode: 'reversible', restore_scope: 'text_and_tools',
  rules: [], custom_rules: [], ttl_seconds: 3600, max_mappings: 256, max_sessions: 10000,
}

function mountCard() {
  return mount(KeyProtectionSettings, { global: { stubs: { OpenAIFastPolicyUserSelector: true } } })
}

describe('KeyProtectionSettings', () => {
  beforeEach(() => {
    vi.resetAllMocks()
    getConfig.mockResolvedValue({ ...defaults })
    getGroups.mockResolvedValue([{ id: 7, name: 'Private group' }])
    updateConfig.mockImplementation(async policy => policy)
  })

  it('loads disabled by default and saves reversible protection with explicit targeting', async () => {
    const wrapper = mountCard()
    await flushPromises()
    expect(wrapper.get('[role="switch"]').attributes('aria-checked')).toBe('false')
    await wrapper.get('[role="switch"]').trigger('click')
    await wrapper.get('[data-testid="protection-groups"]').setValue(['7'])
    await wrapper.get('[data-testid="protection-save"]').trigger('click')
    await flushPromises()
    expect(updateConfig).toHaveBeenCalledWith({ ...defaults, enabled: true, group_ids: [7] })
    expect(showSuccess).toHaveBeenCalledOnce()
  })

  it('does not replace an unreadable policy with disabled defaults', async () => {
    getConfig.mockRejectedValue(new Error('offline'))
    const wrapper = mountCard()
    await flushPromises()
    expect(wrapper.find('[data-testid="protection-save"]').exists()).toBe(false)
    expect(wrapper.get('[role="alert"]').text()).toContain('loadFailed')
    expect(updateConfig).not.toHaveBeenCalled()
  })

  it('rejects malformed custom rules before a save and preserves unknown saved group IDs', async () => {
    getConfig.mockResolvedValue({ ...defaults, group_ids: [123] })
    const wrapper = mountCard()
    await flushPromises()
    expect(wrapper.get('option[value="123"]').text()).toContain('#123')
    await wrapper.get('[data-testid="protection-custom-rules"]').setValue('[{"name":"example"}]')
    await wrapper.get('[data-testid="protection-save"]').trigger('click')
    expect(updateConfig).not.toHaveBeenCalled()
    expect(wrapper.get('[role="alert"]').text()).toContain('invalidRules')
  })

  it('requires explicit confirmation before clearing all users mappings', async () => {
    const wrapper = mountCard()
    await flushPromises()
    const clear = wrapper.findAll('button').find(button => button.text().includes('clearMappings'))!
    await clear.trigger('click')
    expect(clearMappings).not.toHaveBeenCalled()
    await wrapper.findAll('button').find(button => button.text().includes('confirmClear'))!.trigger('click')
    await flushPromises()
    expect(clearMappings).toHaveBeenCalledOnce()
  })

  it('explains the deployment key requirement when reversible mode cannot be enabled', async () => {
    updateConfig.mockRejectedValue({ reason: 'KEY_PROTECTION_PLATFORM_KEY_REQUIRED' })
    const wrapper = mountCard()
    await flushPromises()
    await wrapper.get('[role="switch"]').trigger('click')
    await wrapper.get('[data-testid="protection-save"]').trigger('click')
    await flushPromises()
    expect(wrapper.get('[role="alert"]').text()).toContain('platformKeyRequired')
    expect(showSuccess).not.toHaveBeenCalled()
  })
})
