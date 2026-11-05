import { enableAutoUnmount, flushPromises, mount } from '@vue/test-utils'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import ClientExportConfigFields from '../ClientExportConfigFields.vue'
import Select from '@/components/common/Select.vue'

const { load } = vi.hoisted(() => ({ load: vi.fn() }))
vi.mock('@/api/admin/groups', () => ({ groupsAPI: { getClientExportOptions: load } }))
vi.mock('vue-i18n', () => ({ useI18n: () => ({ t: (key: string) => key }) }))
enableAutoUnmount(afterEach)

const options = { codex_models: ['gpt-5.6-sol', 'gpt-5.6-luna'], gemini_models: ['gemini-2.5-pro'], grok_models: ['grok-4.5'] }

/** Mount real select controls so tests distinguish filtering from free-form entry. */
function mountFields(extra = {}) {
  return mount(ClientExportConfigFields, {
    props: { modelValue: {}, groupId: 42, platform: 'openai', allowlist: { enabled: false, models: [] }, idPrefix: 'test', ...extra },
    attachTo: document.body
  })
}

describe('ClientExportConfigFields', () => {
  beforeEach(() => { load.mockReset(); load.mockResolvedValue(options) })

  it('shows native client first and only one model slot per client tab', async () => {
    const wrapper = mountFields()
    await flushPromises()
    expect(wrapper.findAll('[data-testid=export-codex_model]')).toHaveLength(1)
    expect(wrapper.get('[role=tab][aria-selected=true]').text()).toContain('clients.codex')
    await wrapper.setProps({ platform: 'antigravity' })
    await flushPromises()
    expect(wrapper.find('[data-testid=export-gemini_model]').exists()).toBe(false)
    expect(wrapper.get('[role=tab][aria-selected=true]').text()).toContain('clients.claude')
    await wrapper.findAll('[role=tab]').find(tab => tab.text().includes('clients.gemini'))!.trigger('click')
    expect(wrapper.findAll('[data-testid=export-gemini_model]')).toHaveLength(1)
    expect(wrapper.find('[data-testid=export-codex_model]').exists()).toBe(false)
    await wrapper.setProps({ platform: 'typesafe' })
    await flushPromises()
    expect(wrapper.findAllComponents(Select)).toHaveLength(0)
  })

  it('filters disabled models and rejects arbitrary option events', async () => {
    const wrapper = mountFields({ allowlist: { enabled: true, models: ['gpt-5.6-sol'] } })
    await flushPromises()
    const select = wrapper.findAllComponents(Select).find(select => select.attributes('data-testid') === 'export-codex_model')!
    expect(select.props('options')).toEqual([{ value: '', label: 'admin.clientExport.nativeDefault' }, { value: 'gpt-5.6-sol', label: 'gpt-5.6-sol' }])
    select.vm.$emit('update:modelValue', 'manually-invented-model')
    expect(wrapper.emitted('update:modelValue')).toBeUndefined()
    select.vm.$emit('update:modelValue', 'gpt-5.6-sol')
    expect(wrapper.emitted('update:modelValue')?.[0]).toEqual([{ codex_model: 'gpt-5.6-sol' }])
  })

  it('keeps a removed saved model visible and invalid instead of silently replacing it', async () => {
    const wrapper = mountFields({ modelValue: { codex_model: 'retired-model' } })
    await flushPromises()
    expect(wrapper.get('[role="alert"]').text()).toBe('admin.clientExport.invalid')
    expect(wrapper.emitted('valid')?.at(-1)).toEqual([false])
    expect(wrapper.emitted('update:modelValue')).toBeUndefined()
  })

  it('reports catalog failure, supports retry and allows restoring the native default', async () => {
    load.mockRejectedValueOnce(new Error('offline'))
    const wrapper = mountFields({ modelValue: { codex_model: 'gpt-5.6-sol' } })
    await flushPromises()
    expect(wrapper.text()).toContain('admin.clientExport.loadFailed')
    expect(wrapper.emitted('valid')?.at(-1)).toEqual([false])
    await wrapper.get('button.underline').trigger('click')
    await flushPromises()
    expect(wrapper.emitted('valid')?.at(-1)).toEqual([true])
  })

  it('discards stale responses after switching groups', async () => {
    let finish: (value: typeof options) => void = () => {}
    load.mockReturnValueOnce(new Promise(resolve => { finish = resolve }))
    const wrapper = mountFields()
    await wrapper.setProps({ groupId: 43 })
    await flushPromises()
    finish({ ...options, codex_models: ['private-from-old-group'] })
    await flushPromises()
    expect(JSON.stringify(wrapper.findAllComponents(Select).find(select => select.attributes('data-testid') === 'export-codex_model')!.props('options'))).not.toContain('private-from-old-group')
  })

  it('keeps Gemini and Codex defaults independent and permits presets only', async () => {
    const wrapper = mountFields({ platform: 'gemini', modelValue: { gemini_model: 'gemini-2.5-pro', codex_model: 'gpt-5.6-sol' } })
    await flushPromises()
    expect(wrapper.get('[role=tab][aria-selected=true]').text()).toContain('clients.gemini')
    expect(wrapper.find('[data-testid=export-codex_model]').exists()).toBe(false)
    await wrapper.findAll('[role=tab]').find(tab => tab.text().includes('clients.codex'))!.trigger('click')
    expect(wrapper.find('[data-testid=export-gemini_model]').exists()).toBe(false)
    expect(wrapper.find('[data-testid=export-codex_auth_mode]').exists()).toBe(false)
    const effort = wrapper.findAllComponents(Select).find(select => select.attributes('data-testid') === 'export-codex_reasoning_effort')!
    effort.vm.$emit('update:modelValue', 'custom')
    expect(wrapper.emitted('update:modelValue')).toBeUndefined()
    effort.vm.$emit('update:modelValue', 'high')
    expect(wrapper.emitted('update:modelValue')?.[0]).toEqual([{ gemini_model: 'gemini-2.5-pro', codex_model: 'gpt-5.6-sol', codex_reasoning_effort: 'high' }])
    await wrapper.findAll('[role=tab]').find(tab => tab.text().includes('clients.ccswitch'))!.trigger('click')
    expect(wrapper.find('[data-testid=export-cc_usage_mode]').exists()).toBe(true)
    expect(wrapper.find('[data-testid=export-cc_usage_interval]').exists()).toBe(true)
    expect(wrapper.find('textarea').exists()).toBe(false)
  })

  it('only exposes OpenAI Claude settings when Messages dispatch is enabled', async () => {
    const wrapper = mountFields()
    await flushPromises()
    expect(wrapper.findAll('[role=tab]').some(tab => tab.text().includes('clients.claude'))).toBe(false)
    await wrapper.setProps({ allowMessagesDispatch: true })
    await wrapper.findAll('[role=tab]').find(tab => tab.text().includes('clients.claude'))!.trigger('click')
    expect(wrapper.find('[data-testid=export-claude_traffic]').exists()).toBe(true)
    await wrapper.setProps({ allowMessagesDispatch: false })
    expect(wrapper.get('[role=tab][aria-selected=true]').text()).toContain('clients.codex')
  })
})
