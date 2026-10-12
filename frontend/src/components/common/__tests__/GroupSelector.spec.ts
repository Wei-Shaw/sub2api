import { mount } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import GroupSelector from '../GroupSelector.vue'

const authState = { isSimpleMode: false }

vi.mock('@/stores', () => ({ useAuthStore: () => authState }))
vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return { ...actual, useI18n: () => ({ t: (key: string) => key }) }
})

const groups = [
  { id: 1, name: 'Basic', platform: 'anthropic', status: 'active' },
  { id: 2, name: 'Composite', platform: 'composite', status: 'active' }
] as any

const mountSelector = (modelValue: number[] = []) => mount(GroupSelector, {
  props: { modelValue, groups },
  global: { stubs: { GroupBadge: { props: ['name'], template: '<span>{{ name }}</span>' }, Icon: true } }
})

describe('GroupSelector simple-mode binding policy', () => {
  beforeEach(() => { authState.isSimpleMode = false })

  it('hides composite groups in simple mode and preserves basic groups', () => {
    authState.isSimpleMode = true
    const wrapper = mountSelector()
    expect(wrapper.text()).toContain('Basic')
    expect(wrapper.text()).not.toContain('Composite')
  })

  it('keeps composite groups available in advanced mode', () => {
    const wrapper = mountSelector()
    expect(wrapper.text()).toContain('Composite')
  })

  it('waits for group data before removing known composite selections', async () => {
    authState.isSimpleMode = true
    const wrapper = mount(GroupSelector, {
      props: { modelValue: [1, 2, 99], groups: [] },
      global: { stubs: { GroupBadge: true, Icon: true } }
    })
    expect(wrapper.emitted('update:modelValue')).toBeUndefined()
    await wrapper.setProps({ groups })
    expect(wrapper.emitted('update:modelValue')).toEqual([[[1, 99]]])
    wrapper.unmount()
  })

  it('does not remove selections merely hidden by platform or search filters', async () => {
    authState.isSimpleMode = true
    const wrapper = mountSelector([1, 99])
    await wrapper.setProps({ platform: 'openai', searchable: true })
    await wrapper.get('input[type="text"]').setValue('no-match')
    expect(wrapper.emitted('update:modelValue')).toBeUndefined()
    wrapper.unmount()
  })

  it('cleans hidden historical composite IDs while preserving visible selections', () => {
    authState.isSimpleMode = true
    const wrapper = mountSelector([1, 2])
    expect(wrapper.emitted('update:modelValue')).toEqual([[[1]]])
  })
})
