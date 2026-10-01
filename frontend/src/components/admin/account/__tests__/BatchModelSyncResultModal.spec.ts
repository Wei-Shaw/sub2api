import { describe, expect, it, vi } from 'vitest'
import { mount } from '@vue/test-utils'
import BatchModelSyncResultModal from '../BatchModelSyncResultModal.vue'

vi.mock('vue-i18n', () => ({ useI18n: () => ({ t: (key: string) => key }) }))

const results = [
  { account_id: 1, name: '已同步账号', status: 'success' as const, model_count: 3, added_count: 1, mapping_unchanged: false },
  { account_id: 2, name: '部分能力', status: 'warning' as const, model_count: 3, added_count: 0, mapping_unchanged: true, warnings: [{ code: 'upstream_model_metadata_partial', message: 'partial' }] },
  { account_id: 3, name: '未知结果', status: 'unknown' as const, model_count: 0, added_count: 0, mapping_unchanged: true },
  { account_id: 4, name: '不支持账号', status: 'unsupported' as const, model_count: 0, added_count: 0, mapping_unchanged: true, error: 'Unsupported type' }
]
const mountModal = (running = false) => mount(BatchModelSyncResultModal, {
  props: { show: true, running, results },
  global: { stubs: { BaseDialog: { template: '<div><slot /><slot name="footer" /></div>' } } }
})

describe('BatchModelSyncResultModal', () => {
  it('shows per-account outcomes, metadata warnings and unconfirmed request guidance', async () => {
    const wrapper = mountModal()
    for (const row of results) expect(wrapper.text()).toContain(row.name)
    expect(wrapper.text()).toContain('admin.accounts.syncUpstreamModelsMetadataPartial')
    expect(wrapper.text()).toContain('admin.accounts.batchModelSync.unknownHint')
    expect(wrapper.text()).toContain('Unsupported type')
    expect(wrapper.get('progress').attributes('value')).toBe('4')
    await wrapper.findAll('button').find(button => button.text().includes('selectFailed'))!.trigger('click')
    expect(wrapper.emitted('select-failed')).toHaveLength(1)
  })

  it('offers stop while running instead of selection actions', async () => {
    const wrapper = mountModal(true)
    expect(wrapper.findAll('button')).toHaveLength(1)
    await wrapper.get('button').trigger('click')
    expect(wrapper.emitted('stop')).toHaveLength(1)
    expect(wrapper.emitted('select-failed')).toBeUndefined()
  })
})
