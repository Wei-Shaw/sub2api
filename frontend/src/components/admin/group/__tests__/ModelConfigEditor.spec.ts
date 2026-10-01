import { flushPromises, mount } from '@vue/test-utils'
import { defineComponent, h, ref, type Ref } from 'vue'
import type { ModelAllowlist } from '@/types'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import ModelConfigEditor from '../ModelConfigEditor.vue'
import { mergeModelFields, parseModelFields } from '../modelConfig'
import { getCodexModelConfig, importCodexModelConfig, getModelConfigCatalog } from '@/api/admin/groups'

vi.mock('vue-i18n', () => ({ useI18n: () => ({ t: (key: string) => key }) }))
vi.mock('@/api/admin/groups', () => ({ getCodexModelConfig: vi.fn(), importCodexModelConfig: vi.fn(), getModelConfigCatalog: vi.fn() }))

const mountEditor = (allowlist?: Ref<ModelAllowlist>) => mount(defineComponent({
  setup() {
    const overrides = ref({})
    return () => h(ModelConfigEditor, { groupId: 7, modelAllowlist: allowlist?.value, modelValue: overrides.value, 'onUpdate:modelValue': value => { overrides.value = value } })
  }
}))
const click = async (wrapper: ReturnType<typeof mountEditor>, key: string) => {
  const button = key === 'search' ? wrapper.get('[data-testid="model-config-search"]') : wrapper.findAll('button').find(button => button.text() === `modelConfig.${key}`)!
  await button.trigger('click')
  await flushPromises()
}

beforeEach(() => {
  vi.resetAllMocks()
  vi.mocked(getModelConfigCatalog).mockResolvedValue([])
  vi.mocked(getCodexModelConfig).mockResolvedValue([{ slug: 'MiniMax-M3', display_name: 'MiniMax', context_window: 272000 }, { slug: 'k3' }])
})
afterEach(() => vi.useRealTimers())

describe('ModelConfigEditor', () => {
  it('edits input modalities via checkboxes without changing unrelated fields or allowing an empty selection', async () => {
    vi.mocked(getCodexModelConfig).mockResolvedValue([{ slug: 'k3', input_modalities: ['text'] }])
    const wrapper = mountEditor()
    await click(wrapper, 'load')
    const text = wrapper.get<HTMLInputElement>('[data-testid="model-config-modality-text"]')
    const image = wrapper.get<HTMLInputElement>('[data-testid="model-config-modality-image"]')
    expect(text.element.checked).toBe(true)
    expect(text.element.disabled).toBe(true)
    expect(image.element.checked).toBe(false)
    expect(wrapper.findComponent(ModelConfigEditor).emitted('update:modelValue')).toBeUndefined()
    await wrapper.get('textarea').setValue('{"description":"Keep description","supports_search_tool":false}')
    await image.setValue(true)
    expect(JSON.parse(wrapper.get('textarea').element.value)).toEqual({ description: 'Keep description', supports_search_tool: false, input_modalities: ['text', 'image'] })
    expect(text.element.disabled).toBe(false)
    await text.setValue(false)
    expect(JSON.parse(wrapper.get('textarea').element.value).input_modalities).toEqual(['image'])
    expect(image.element.disabled).toBe(true)
    expect(wrapper.get('pre').text()).toContain('"image"')
    wrapper.unmount()
  })
  it('reflects input modalities from JSON and upstream imports and restores inherited values', async () => {
    vi.mocked(getCodexModelConfig).mockResolvedValue([{ slug: 'k3', input_modalities: ['text', 'image'] }])
    const wrapper = mountEditor()
    await click(wrapper, 'load')
    const image = wrapper.get<HTMLInputElement>('[data-testid="model-config-modality-image"]')
    expect(image.element.checked).toBe(true)
    await wrapper.get('textarea').setValue('{"input_modalities":["text"]}')
    expect(image.element.checked).toBe(false)
    await click(wrapper, 'reset')
    expect(image.element.checked).toBe(true)
    expect(wrapper.get('textarea').element.value).toBe('{}')
    vi.mocked(importCodexModelConfig).mockResolvedValue({ input_modalities: ['text'] })
    await click(wrapper, 'upstream')
    expect(image.element.checked).toBe(false)
    expect(wrapper.get<HTMLInputElement>('[data-testid="model-config-modality-text"]').element.disabled).toBe(true)
    wrapper.unmount()
  })
  it('refreshes model choices using the unsaved allowlist and preserves edited model drafts', async () => {
    vi.useFakeTimers()
    const allowlist = ref<ModelAllowlist>({ enabled: true, models: ['k3'] })
    vi.mocked(getCodexModelConfig).mockImplementation(async (_id, _signal, draft) =>
      (draft?.enabled ? draft.models : ['k3', 'MiniMax-M3']).map(slug => ({ slug })))
    const wrapper = mountEditor(allowlist)
    await click(wrapper, 'load')
    expect(getCodexModelConfig).toHaveBeenLastCalledWith(7, expect.any(AbortSignal), { enabled: true, models: ['k3'] })
    await wrapper.get('textarea').setValue('{"description":"Keep my edits"}')
    allowlist.value = { enabled: true, models: ['MiniMax-M3', 'k3'] }
    await vi.advanceTimersByTimeAsync(200)
    expect(wrapper.get('select').findAll('option').map(option => option.attributes('value'))).toEqual(['MiniMax-M3', 'k3'])
    expect(wrapper.get('select').element.value).toBe('k3')
    expect(JSON.parse(wrapper.get('textarea').element.value).description).toBe('Keep my edits')
    allowlist.value = { enabled: true, models: ['MiniMax-M3'] }
    await vi.advanceTimersByTimeAsync(200)
    expect(wrapper.get('select').element.value).toBe('MiniMax-M3')
    allowlist.value = { enabled: true, models: ['k3', 'MiniMax-M3'] }
    await vi.advanceTimersByTimeAsync(200)
    await wrapper.get('select').setValue('k3')
    expect(JSON.parse(wrapper.get('textarea').element.value).description).toBe('Keep my edits')
    allowlist.value = { enabled: true, models: [] }
    await vi.advanceTimersByTimeAsync(200)
    expect(wrapper.find('select').exists()).toBe(false)
    expect(wrapper.text()).toContain('modelConfig.empty')
    allowlist.value = { enabled: false, models: [] }
    await vi.advanceTimersByTimeAsync(200)
    expect(wrapper.get('select').findAll('option')).toHaveLength(2)
    expect(getModelConfigCatalog).toHaveBeenCalledTimes(1)
    wrapper.unmount()
  })
  it('ignores old allowlist preview responses and imports using the current draft', async () => {
    vi.useFakeTimers()
    const allowlist = ref<ModelAllowlist>({ enabled: true, models: ['k3'] })
    let finishOld!: (value: Array<{slug: string}>) => void
    vi.mocked(getCodexModelConfig).mockReturnValueOnce(new Promise(resolve => { finishOld = resolve }))
    const wrapper = mountEditor(allowlist)
    await click(wrapper, 'load')
    const oldSignal = vi.mocked(getCodexModelConfig).mock.calls[0][1]!
    allowlist.value = { enabled: true, models: ['MiniMax-M3'] }
    vi.mocked(getCodexModelConfig).mockResolvedValue([{slug: 'MiniMax-M3'}])
    await vi.advanceTimersByTimeAsync(200)
    expect(oldSignal.aborted).toBe(true)
    finishOld([{slug: 'k3'}])
    await flushPromises()
    expect(wrapper.get('select').element.value).toBe('MiniMax-M3')
    vi.mocked(importCodexModelConfig).mockResolvedValue({ description: 'New model' })
    await click(wrapper, 'upstream')
    expect(importCodexModelConfig).toHaveBeenLastCalledWith(7, 'MiniMax-M3', expect.any(AbortSignal), {enabled: true, models: ['MiniMax-M3']})
    wrapper.unmount()
  })
  it('shows reasoning levels and exact context windows for all matching entries without filtering', async () => {
    vi.mocked(getModelConfigCatalog).mockResolvedValue([
      { id: 'm3', provider: 'with-levels', name: 'Explicit levels', fields: { context_window: 1048576, supported_reasoning_levels: [{ effort: 'low' }, { effort: 'high' }] } },
      { id: 'm3', provider: 'toggle-only', name: 'Toggle only', fields: { context_window: 1000000 } },
      { id: 'm3', provider: 'missing', name: 'Missing metadata', fields: {} },
      { id: 'm3', provider: 'none', name: 'Reasoning disabled', fields: { supported_reasoning_levels: [{ effort: 'none' }] } },
    ])
    const wrapper = mountEditor()
    await click(wrapper, 'load')
    expect(wrapper.find('[data-testid="model-config-reasoning-filter"]').exists()).toBe(false)
    await wrapper.get('input').setValue('m3')
    await click(wrapper, 'search')
    expect(getModelConfigCatalog).toHaveBeenCalledTimes(1)
    expect(getModelConfigCatalog).toHaveBeenLastCalledWith(expect.any(AbortSignal))
    const explicit = wrapper.findAll('button').find(button => button.text().includes('Explicit levels'))!
    expect(explicit.text()).toContain('low, high')
    expect(explicit.text()).toContain('1,048,576 tokens')
    const toggle = wrapper.findAll('button').find(button => button.text().includes('Toggle only'))!
    expect(toggle.text()).toContain('modelConfig.reasoningLevelsMissing')
    expect(toggle.text()).toContain('1,000,000 tokens')
    const missing = wrapper.findAll('button').find(button => button.text().includes('Missing metadata'))!
    expect(missing.text()).toContain('modelConfig.notProvided')
    const disabled = wrapper.findAll('button').find(button => button.text().includes('Reasoning disabled'))!
    expect(disabled.text()).toContain('modelConfig.supported_reasoning_levels: none')
    expect(wrapper.get('textarea').element.value).toBe('{}')
    wrapper.unmount()
  })
  it('does not present unknown or invalid context windows as zero-sized windows', async () => {
    vi.mocked(getModelConfigCatalog).mockResolvedValue([
      { id: 'zero', provider: 'test', name: 'Zero context', fields: { context_window: 0 } },
      { id: 'null', provider: 'test', name: 'Null context', fields: { context_window: null } },
    ])
    const wrapper = mountEditor()
    await click(wrapper, 'load')
    await wrapper.get('input').setValue('context')
    await click(wrapper, 'search')
    for (const name of ['Zero context', 'Null context']) {
      const button = wrapper.findAll('button').find(button => button.text().includes(name))!
      expect(button.text()).toContain('modelConfig.notProvided')
      expect(button.text()).not.toContain('0 tokens')
    }
    wrapper.unmount()
  })
  it('edits reasoning levels with checkboxes while preserving upstream metadata and other fields', async () => {
    const upstreamHigh = { effort: 'high', description: 'Provider high effort', future_option: true }
    vi.mocked(getCodexModelConfig).mockResolvedValue([{
      slug: 'MiniMax-M3', default_reasoning_level: 'high',
      supported_reasoning_levels: [{ effort: 'low', description: 'Provider low effort' }, upstreamHigh, { effort: 'custom', description: 'Custom effort' }],
    }])
    const wrapper = mountEditor()
    await click(wrapper, 'load')
    const high = wrapper.get<HTMLInputElement>('input[type="checkbox"][value="high"]')
    expect(high.element.checked).toBe(true)
    expect(wrapper.get<HTMLInputElement>('input[value="custom"]').element.checked).toBe(true)
    expect(wrapper.findComponent(ModelConfigEditor).emitted('update:modelValue')).toBeUndefined()
    await wrapper.get('textarea').setValue('{"description":"My model"}')
    await high.setValue(false)
    let fields = JSON.parse(wrapper.get('textarea').element.value)
    expect(fields.description).toBe('My model')
    expect(fields.supported_reasoning_levels.map((level: { effort: string }) => level.effort)).toEqual(['low', 'custom'])
    expect(fields.default_reasoning_level).toBe('low')
    await high.setValue(true)
    fields = JSON.parse(wrapper.get('textarea').element.value)
    expect(fields.supported_reasoning_levels).toContainEqual(upstreamHigh)
    const defaultSelect = wrapper.get('[data-testid="model-config-default-reasoning"]')
    expect(defaultSelect.findAll('option').map(option => option.attributes('value'))).toEqual(['', 'low', 'custom', 'high'])
    await defaultSelect.setValue('custom')
    expect(JSON.parse(wrapper.get('textarea').element.value).default_reasoning_level).toBe('custom')
    wrapper.unmount()
  })
  it('writes an explicit empty list and null default when every reasoning level is unchecked', async () => {
    vi.mocked(getCodexModelConfig).mockResolvedValue([{
      slug: 'k3', default_reasoning_level: 'high', supported_reasoning_levels: [{ effort: 'high', description: 'High' }],
    }])
    const wrapper = mountEditor()
    await click(wrapper, 'load')
    await wrapper.get('input[type="checkbox"][value="high"]').setValue(false)
    expect(JSON.parse(wrapper.get('textarea').element.value)).toEqual({ supported_reasoning_levels: [], default_reasoning_level: null })
    expect(wrapper.get('[data-testid="model-config-default-reasoning"]').attributes('disabled')).toBeDefined()
    await wrapper.get('input[type="checkbox"][value="max"]').setValue(true)
    expect(JSON.parse(wrapper.get('textarea').element.value)).toEqual({ supported_reasoning_levels: [{ effort: 'max', description: 'max' }], default_reasoning_level: 'max' })
    wrapper.unmount()
  })
  it('reflects JSON/import edits in checkboxes and restores inherited reasoning on reset', async () => {
    vi.mocked(getCodexModelConfig).mockResolvedValue([{
      slug: 'k3', default_reasoning_level: 'low', supported_reasoning_levels: [{ effort: 'low', description: 'Low' }],
    }])
    const wrapper = mountEditor()
    await click(wrapper, 'load')
    await wrapper.get('textarea').setValue('{"supported_reasoning_levels":[{"effort":"ultra","description":"Ultra"}],"default_reasoning_level":"ultra"}')
    expect(wrapper.get<HTMLInputElement>('input[value="ultra"]').element.checked).toBe(true)
    expect(wrapper.get<HTMLInputElement>('input[value="low"]').element.checked).toBe(false)
    await click(wrapper, 'reset')
    expect(wrapper.get<HTMLInputElement>('input[value="low"]').element.checked).toBe(true)
    expect(wrapper.get<HTMLInputElement>('input[value="ultra"]').element.checked).toBe(false)
    expect(wrapper.get('textarea').element.value).toBe('{}')
    vi.mocked(importCodexModelConfig).mockResolvedValue({ supported_reasoning_levels: [{ effort: 'xhigh', description: 'Imported' }], default_reasoning_level: 'xhigh' })
    await click(wrapper, 'upstream')
    expect(wrapper.get<HTMLInputElement>('input[value="xhigh"]').element.checked).toBe(true)
    expect(wrapper.get<HTMLInputElement>('input[value="low"]').element.checked).toBe(false)
    wrapper.unmount()
  })
  it('downloads the complete catalogue on mount and filters immediately without further requests', async () => {
    vi.mocked(getModelConfigCatalog).mockResolvedValue([
      { id: 'MiniMax-M3', provider: 'minimax', provider_name: 'MiniMax Provider', name: 'MiniMax model', fields: {} },
      { id: 'k3', provider: 'kimi', name: 'Kimi model', fields: {} },
      ...Array.from({ length: 250 }, (_, i) => ({ id: `rare-id-${i}`, provider: 'large', name: `Entry ${i}`, fields: {} })),
    ])
    const wrapper = mountEditor()
    expect(getModelConfigCatalog).toHaveBeenCalledTimes(1)
    await click(wrapper, 'load')
    await wrapper.get('input').setValue('minimax provider')
    expect(wrapper.text()).toContain('MiniMax model')
    expect(wrapper.text()).not.toContain('Kimi model')
    await wrapper.get('input').setValue('k3')
    expect(wrapper.text()).toContain('Kimi model')
    expect(wrapper.text()).not.toContain('MiniMax model')
    await wrapper.get('input').setValue('rare-id-249')
    expect(wrapper.text()).toContain('Entry 249')
    await wrapper.get('input').trigger('keydown', { key: 'Enter' })
    await click(wrapper, 'search')
    expect(getModelConfigCatalog).toHaveBeenCalledTimes(1)
    wrapper.unmount()
  })
  it('uses the latest query when the initial catalogue download finishes', async () => {
    let resolve!: (value: Awaited<ReturnType<typeof getModelConfigCatalog>>) => void
    vi.mocked(getModelConfigCatalog).mockReturnValue(new Promise(done => { resolve = done }))
    const wrapper = mountEditor()
    await click(wrapper, 'load')
    await wrapper.get('input').setValue('old')
    await wrapper.get('input').setValue('new')
    expect(wrapper.text()).not.toContain('modelConfig.noResults')
    expect(getModelConfigCatalog).toHaveBeenCalledTimes(1)
    resolve([
      { id: 'old', provider: 'p', name: 'Previous result', fields: {} },
      { id: 'new', provider: 'p', name: 'Latest result', fields: {} },
    ])
    await flushPromises()
    expect(wrapper.text()).toContain('Latest result')
    expect(wrapper.text()).not.toContain('Previous result')
    wrapper.unmount()
  })
  it('clears local matches and reuses the same catalogue after switching models', async () => {
    vi.mocked(getModelConfigCatalog).mockResolvedValue([{ id: 'm3', name: 'Found model', provider: 'minimax', fields: {} }])
    const wrapper = mountEditor()
    await click(wrapper, 'load')
    await wrapper.get('input').setValue('m3')
    expect(wrapper.text()).toContain('Found model')
    await wrapper.get('input').setValue('   ')
    expect(wrapper.text()).not.toContain('Found model')
    expect(wrapper.text()).not.toContain('modelConfig.noResults')
    await wrapper.get('select').setValue('k3')
    await wrapper.get('input').setValue('m3')
    expect(wrapper.text()).toContain('Found model')
    expect(getModelConfigCatalog).toHaveBeenCalledTimes(1)
    wrapper.unmount()
  })
  it('allows an explicit retry after failure instead of retrying on every keystroke', async () => {
    vi.mocked(getModelConfigCatalog).mockRejectedValueOnce(new Error('offline')).mockResolvedValue([{ id: 'm3', provider: 'minimax', name: 'Recovered catalogue', fields: {} }])
    const wrapper = mountEditor()
    await click(wrapper, 'load')
    expect(wrapper.text()).toContain('modelConfig.searchError')
    await wrapper.get('input').setValue('mini')
    await wrapper.get('input').setValue('m3')
    expect(getModelConfigCatalog).toHaveBeenCalledTimes(1)
    await wrapper.get('[data-testid="model-config-catalog-retry"]').trigger('click')
    await flushPromises()
    expect(getModelConfigCatalog).toHaveBeenCalledTimes(2)
    expect(wrapper.text()).toContain('Recovered catalogue')
    wrapper.unmount()
  })
  it('cancels the pending catalogue request when the dialog closes', async () => {
    let resolve!: (value: Awaited<ReturnType<typeof getModelConfigCatalog>>) => void
    vi.mocked(getModelConfigCatalog).mockReturnValueOnce(new Promise(done => { resolve = done }))
    const wrapper = mountEditor()
    const signal = vi.mocked(getModelConfigCatalog).mock.calls[0][0]!
    wrapper.unmount()
    expect(signal.aborted).toBe(true)
    resolve([])
    await flushPromises()
    const reopened = mountEditor()
    expect(getModelConfigCatalog).toHaveBeenCalledTimes(2)
    reopened.unmount()
  })
  it('preserves nested fields and explicit false/null/empty arrays', () => {
    expect(mergeModelFields({ model_messages: { instructions_template: 'keep', permissions: {} }, enabled: true }, { model_messages: { permissions: null }, enabled: false, tools: [] }))
      .toEqual({ model_messages: { instructions_template: 'keep', permissions: null }, enabled: false, tools: [] })
    expect(() => parseModelFields('{"slug":"new"}')).toThrow()
    expect(() => parseModelFields('[]')).toThrow()
  })
  it('imports a registry model into the selected ID and keeps unrelated edits', async () => {
    vi.mocked(getModelConfigCatalog).mockResolvedValue([{ id: 'provider/M3', provider: 'minimax', name: 'Registry M3', fields: { context_window: 1000000, input_modalities: ['text', 'image'] } }])
    const wrapper = mountEditor()
    await click(wrapper, 'load')
    await wrapper.get('textarea').setValue('{"description":"mine","supports_search_tool":false}')
    await wrapper.get('input').setValue('M3')
    await click(wrapper, 'search')
    await wrapper.findAll('button').find(button => button.text().includes('Registry M3'))!.trigger('click')
    const fields = JSON.parse(wrapper.get('textarea').element.value)
    expect(fields).toMatchObject({ description: 'mine', supports_search_tool: false, context_window: 1000000 })
    expect(fields.slug).toBeUndefined()
    expect(wrapper.get('select').element.value).toBe('MiniMax-M3')
    wrapper.unmount()
  })
  it('preserves manual fields on upstream import and blocks invalid JSON saving', async () => {
    const wrapper = mountEditor()
    await click(wrapper, 'load')
    await wrapper.get('textarea').setValue('{"description":"manual"}')
    vi.mocked(importCodexModelConfig).mockResolvedValue({ description: 'upstream', model_messages: { instructions_template: 'upstream template' } })
    await click(wrapper, 'upstream')
    expect(JSON.parse(wrapper.get('textarea').element.value)).toEqual({ description: 'manual', model_messages: { instructions_template: 'upstream template' } })
    await wrapper.get('textarea').setValue('{broken')
    expect((wrapper.findComponent(ModelConfigEditor).vm as unknown as { validate: () => boolean }).validate()).toBe(false)
    await wrapper.get('select').setValue('k3')
    expect(wrapper.get('select').element.value).toBe('MiniMax-M3')
    await click(wrapper, 'reset')
    expect(wrapper.get('textarea').element.value).toBe('{}')
    wrapper.unmount()
  })
  it('ignores an upstream response after switching models', async () => {
    let resolve!: (value: Record<string, unknown>) => void
    vi.mocked(importCodexModelConfig).mockReturnValue(new Promise(done => { resolve = done }))
    const wrapper = mountEditor()
    await click(wrapper, 'load')
    await click(wrapper, 'upstream')
    await wrapper.get('select').setValue('k3')
    resolve({ description: 'wrong model' })
    await flushPromises()
    expect(wrapper.get('textarea').element.value).toBe('{}')
    wrapper.unmount()
  })
})
