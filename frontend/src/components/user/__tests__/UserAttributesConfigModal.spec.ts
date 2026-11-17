import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { enableAutoUnmount, flushPromises, mount } from '@vue/test-utils'
import UserAttributesConfigModal from '../UserAttributesConfigModal.vue'

const mocks = vi.hoisted(() => ({ listDefinitions: vi.fn(), updateDefinition: vi.fn(), createDefinition: vi.fn() }))
vi.mock('@/api/admin', () => ({ adminAPI: { userAttributes: mocks } }))
vi.mock('@/stores/app', () => ({ useAppStore: () => ({ showSuccess: vi.fn(), showError: vi.fn() }) }))
vi.mock('vue-i18n', () => ({ useI18n: () => ({ t: (key: string) => key }) }))
enableAutoUnmount(afterEach)
beforeEach(() => {
  vi.clearAllMocks()
  mocks.listDefinitions.mockResolvedValue([{ id: 1, key: 'note', name: 'Note', type: 'text', description: 'Old description', placeholder: 'Old placeholder', required: false, enabled: true }])
  mocks.updateDefinition.mockResolvedValue(undefined)
  mocks.createDefinition.mockResolvedValue(undefined)
})
async function openConfig() {
  const wrapper = mount(UserAttributesConfigModal, {
    props: { show: false }, global: { stubs: {
      BaseDialog: { props: ['show'], template: '<div v-if="show"><slot /><slot name="footer" /></div>' },
      ConfirmDialog: true, Select: true, Icon: true
    } }
  })
  await wrapper.setProps({ show: true })
  await flushPromises()
  return wrapper
}

describe('UserAttributesConfigModal optional text fields', () => {
  it.each([
    ['', 'Old placeholder'],
    ['Old description', ''],
    ['', ''],
    ['New description', 'New placeholder']
  ])('persists description %j and placeholder %j explicitly', async (description, placeholder) => {
    const wrapper = await openConfig()
    await wrapper.get('button[title="common.edit"]').trigger('click')
    await wrapper.get('input[placeholder="admin.users.attributes.fieldDescriptionHint"]').setValue(description)
    await wrapper.get('input[placeholder="admin.users.attributes.placeholderHint"]').setValue(placeholder)
    await wrapper.get('form').trigger('submit')
    await flushPromises()
    expect(mocks.updateDefinition).toHaveBeenCalledTimes(1)
    const [id, data] = mocks.updateDefinition.mock.calls[0]!
    expect(id).toBe(1)
    expect(JSON.parse(JSON.stringify(data))).toEqual({ key: 'note', name: 'Note', type: 'text', description, placeholder, required: false, enabled: true })
  })

  it('still creates an attribute without optional text', async () => {
    const wrapper = await openConfig()
    await wrapper.findAll('button').find(b => b.text() === 'admin.users.attributes.addAttribute')!.trigger('click')
    await wrapper.get('input[placeholder="admin.users.attributes.keyHint"]').setValue('note')
    await wrapper.get('input[placeholder="admin.users.attributes.nameHint"]').setValue('Note')
    await wrapper.get('form').trigger('submit')
    await flushPromises()
    expect(mocks.createDefinition).toHaveBeenCalledWith(expect.objectContaining({ key: 'note', name: 'Note', type: 'text' }))
    expect(mocks.updateDefinition).not.toHaveBeenCalled()
  })
})
