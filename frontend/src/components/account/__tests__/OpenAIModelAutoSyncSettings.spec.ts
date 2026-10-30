import { describe, expect, it } from 'vitest'
import { mount } from '@vue/test-utils'
import { createI18n } from 'vue-i18n'
import OpenAIModelAutoSyncSettings from '../OpenAIModelAutoSyncSettings.vue'
import enAccounts from '@/i18n/locales/en/admin/accounts'
import zhAccounts from '@/i18n/locales/zh/admin/accounts'
import type { OpenAIModelAutoSyncState } from '@/types'

// Vitest uses vue-i18n's runtime-only build. These messages have no interpolation.
function messageFunctions(messages: Record<string, string>) {
  return Object.fromEntries(Object.entries(messages).map(([key, text]) => [key, () => text]))
}

function mountSettings(props: {
  modelValue: boolean
  state?: OpenAIModelAutoSyncState
  unrestricted?: boolean
  disabled?: boolean
}, locale = 'en') {
  return mount(OpenAIModelAutoSyncSettings, {
    props,
    global: {
      plugins: [createI18n({
        legacy: false,
        locale,
        messages: {
          en: { admin: { accounts: { openai: { modelAutoSync: messageFunctions(enAccounts.accounts.openai.modelAutoSync) } } } },
          zh: { admin: { accounts: { openai: { modelAutoSync: messageFunctions(zhAccounts.accounts.openai.modelAutoSync) } } } }
        }
      })]
    }
  })
}

const messages = enAccounts.accounts.openai.modelAutoSync

describe('OpenAIModelAutoSyncSettings', () => {
  it('provides a labeled checkbox with help text and emits only a draft boolean', async () => {
    const wrapper = mountSettings({ modelValue: false })
    const checkbox = wrapper.get<HTMLInputElement>('input[type="checkbox"]')
    expect(checkbox.element.checked).toBe(false)
    expect(checkbox.element.labels?.[0]?.textContent).toContain(messages.title)
    const help = wrapper.get(`[id="${checkbox.attributes('aria-describedby')}"]`)
    expect(help.text()).toContain(messages.schedule)
    expect(help.text()).toContain(messages.preservation)
    expect(help.text()).toContain(messages.retention)
    expect(wrapper.get('[role="status"]').text()).toContain(messages.disabled)

    await checkbox.setValue(true)
    expect(wrapper.emitted('update:modelValue')).toEqual([[true]])
    expect(wrapper.emitted()).not.toHaveProperty('sync')
    wrapper.unmount()
  })

  it('warns that an unrestricted account becomes a whitelist only while enabling it', async () => {
    const wrapper = mountSettings({ modelValue: true, unrestricted: true })
    expect(wrapper.text()).toContain(messages.unrestrictedHint)
    expect(wrapper.get('[role="status"]').text()).toContain(messages.pending)
    expect(wrapper.get('[role="status"]').text()).toContain(messages.neverSynced)
    await wrapper.setProps({ unrestricted: false })
    expect(wrapper.text()).not.toContain(messages.unrestrictedHint)
    await wrapper.setProps({ unrestricted: true, modelValue: false })
    expect(wrapper.text()).not.toContain(messages.unrestrictedHint)
    wrapper.unmount()
  })

  it('shows the last successful sync time even after the latest attempt failed', () => {
    const lastSuccess = '2026-10-28T12:00:00Z'
    const wrapper = mountSettings({
      modelValue: true,
      state: {
        status: 'error',
        error_code: 'upstream returned private request details',
        last_attempt_at: '2026-10-28T13:00:00Z',
        last_success_at: lastSuccess,
        managed_models: ['private-model-id']
      }
    })
    expect(wrapper.get('[role="status"]').text()).toContain(messages.error)
    expect(wrapper.get('time').attributes('datetime')).toBe(lastSuccess)
    expect(wrapper.get('time').text()).not.toBe('')
    expect(wrapper.text()).not.toContain('private request details')
    expect(wrapper.text()).not.toContain('private-model-id')
    wrapper.unmount()
  })

  it('explains when sync keeps the last model to prevent unrestricted access', () => {
    const wrapper = mountSettings({ modelValue: true, state: { status: 'error', error_code: 'empty_mapping' } })
    expect(wrapper.get('[role="status"]').text()).toContain(messages.emptyMapping)
    expect(wrapper.text()).not.toContain('empty_mapping')
    wrapper.unmount()
  })

  it('handles a missing or invalid success timestamp without rendering an invalid date', () => {
    const wrapper = mountSettings({
      modelValue: true,
      state: { status: 'success', last_success_at: 'invalid timestamp' }
    })
    expect(wrapper.get('[role="status"]').text()).toContain(messages.success)
    expect(wrapper.get('[role="status"]').text()).toContain(messages.neverSynced)
    expect(wrapper.find('time').exists()).toBe(false)
    wrapper.unmount()
  })

  it('disables editing while the parent form is saving', async () => {
    const wrapper = mountSettings({ modelValue: true, disabled: true })
    const checkbox = wrapper.get<HTMLInputElement>('input[type="checkbox"]')
    expect(checkbox.element.disabled).toBe(true)
    await checkbox.trigger('change')
    expect(wrapper.emitted('update:modelValue')).toBeUndefined()
    wrapper.unmount()
  })

  it('renders translated schedule, retention and status in Chinese', () => {
    const wrapper = mountSettings({ modelValue: true, unrestricted: true }, 'zh')
    const localized = zhAccounts.accounts.openai.modelAutoSync
    for (const key of ['title', 'schedule', 'preservation', 'retention', 'unrestrictedHint', 'pending', 'neverSynced'] as const) {
      expect(wrapper.text()).toContain(localized[key])
    }
    wrapper.unmount()
  })
})
