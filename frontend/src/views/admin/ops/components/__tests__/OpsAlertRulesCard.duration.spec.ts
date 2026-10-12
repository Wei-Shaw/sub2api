import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { enableAutoUnmount, flushPromises, shallowMount } from '@vue/test-utils'

vi.mock('vue-i18n', async (importOriginal) => ({
  ...await importOriginal<typeof import('vue-i18n')>(),
  useI18n: () => ({ t: (key: string) => key }),
}))
enableAutoUnmount(afterEach)


import OpsAlertRulesCard from '../OpsAlertRulesCard.vue'
import { ref } from 'vue'
const { createAlertRule } = vi.hoisted(() => ({ createAlertRule: vi.fn() }))
vi.mock('@vueuse/core', () => ({ useMediaQuery: () => ref(true) }))
vi.mock('@/api', () => ({ adminAPI: { groups: { getAll: vi.fn().mockResolvedValue([]) } } }))
vi.mock('@/api/admin/ops', () => ({ opsAPI: { listAlertRules: vi.fn().mockResolvedValue([]), createAlertRule } }))
vi.mock('@/stores/app', () => ({ useAppStore: () => ({ showError: vi.fn(), showSuccess: vi.fn() }) }))

async function openRule() {
  const wrapper = shallowMount(OpsAlertRulesCard, {
    global: { stubs: { BaseDialog: { props: ['show'], template: '<div v-if="show"><slot /><slot name="footer" /></div>' } } },
  })
  await flushPromises()
  await wrapper.findAll('button').find(b => b.text().includes('admin.ops.alertRules.create'))!.trigger('click')
  await wrapper.find('input[type="text"]').setValue('Error rate')
  return wrapper
}
describe('alert rule integer durations', () => {
  beforeEach(() => { vi.clearAllMocks() })
  it.each(['1.5', '0.5'])('rejects fractional duration %s before submitting', async value => {
    const wrapper = await openRule()
    const input = wrapper.find(value === '1.5' ? 'input[min="1"][max="1440"]' : 'input[min="0"][max="1440"]')
    await input.setValue(value)
    const save = wrapper.findAll('button').find(b => b.text() === 'common.save')!
    expect(wrapper.text()).toContain(value === '1.5' ? 'admin.ops.alertRules.validation.sustainedRange' : 'admin.ops.alertRules.validation.cooldownRange')
    await save.trigger('click')
    expect(createAlertRule).not.toHaveBeenCalled()
  })
  it('allows whole-minute boundaries and a fractional metric threshold', async () => {
    const wrapper = await openRule()
    await wrapper.get('input[min="1"][max="1440"]').setValue('1440')
    await wrapper.get('input[min="0"][max="1440"]').setValue('0')
    await wrapper.get('input[type="number"]:not([min])').setValue('1.25')
    const save = wrapper.findAll('button').find(b => b.text() === 'common.save')!
    expect(save.attributes('disabled')).toBeUndefined()
    await save.trigger('click')
    await flushPromises()
    expect(createAlertRule).toHaveBeenCalledWith(expect.objectContaining({ sustained_minutes: 1440, cooldown_minutes: 0, threshold: 1.25 }))
  })
})
