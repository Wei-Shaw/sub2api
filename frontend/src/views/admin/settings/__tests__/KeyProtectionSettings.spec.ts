import { flushPromises, mount } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import KeyProtectionSettings from '../KeyProtectionSettings.vue'

const { getConfig, updateConfig, getGroups, showSuccess } = vi.hoisted(() => ({
  getConfig: vi.fn(), updateConfig: vi.fn(), getGroups: vi.fn(), showSuccess: vi.fn(),
}))

vi.mock('vue-i18n', () => ({ useI18n: () => ({ t: (key: string) => key }) }))
vi.mock('@/api/admin/keyProtection', () => ({
  getKeyProtectionConfig: getConfig,
  updateKeyProtectionConfig: updateConfig,
}))
vi.mock('@/api/admin/groups', () => ({ getAllIncludingInactive: getGroups }))
vi.mock('@/api/admin', () => ({ adminAPI: {} }))
vi.mock('@/stores', () => ({ useAppStore: () => ({ showSuccess }) }))

const defaults = {
  enabled: false, user_ids: [], group_ids: [], rules: [], custom_rules: [],
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

  it('saves a built-in allowlist and adds or deletes custom rules', async () => {
    const wrapper = mountCard()
    await flushPromises()
    const rule = { name: 'fictional', pattern: 'fictional_[a-z]{20}' }
    await wrapper.get('[data-testid="protection-rules"]').setValue('github, anthropic')
    await wrapper.get('[data-testid="protection-custom-rules"]').setValue(JSON.stringify([rule]))
    await wrapper.get('[data-testid="protection-save"]').trigger('click')
    await flushPromises()
    expect(updateConfig).toHaveBeenLastCalledWith({ ...defaults, rules: ['github', 'anthropic'], custom_rules: [rule] })
    await wrapper.get('[data-testid="protection-custom-rules"]').setValue('[]')
    await wrapper.get('[data-testid="protection-save"]').trigger('click')
    await flushPromises()
    expect(updateConfig).toHaveBeenLastCalledWith({ ...defaults, rules: ['github', 'anthropic'] })
  })

  it('keeps edited settings when saving fails', async () => {
    updateConfig.mockRejectedValue(new Error('unavailable'))
    const wrapper = mountCard()
    await flushPromises()
    await wrapper.get('[role="switch"]').trigger('click')
    await wrapper.get('[data-testid="protection-save"]').trigger('click')
    await flushPromises()
    expect(wrapper.get('[role="alert"]').text()).toContain('saveFailed')
    expect(wrapper.get('[role="switch"]').attributes('aria-checked')).toBe('true')
    expect(showSuccess).not.toHaveBeenCalled()
  })
})
