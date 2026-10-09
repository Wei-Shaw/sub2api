import { DOMWrapper, mount } from '@vue/test-utils'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { nextTick } from 'vue'
import Select from '../Select.vue'

vi.mock('vue-i18n', () => ({ useI18n: () => ({ t: (key: string) => key }) }))
let wrapper: ReturnType<typeof mount>
afterEach(() => { wrapper?.unmount(); document.body.innerHTML = '' })

async function open(props: Record<string, unknown> = {}) {
  wrapper = mount(Select, {
    attachTo: document.body,
    props: {
      modelValue: 'gamma', searchable: true,
      options: [
        { value: 'alpha', label: 'Alpha' },
        { value: 'beta', label: 'Beta' },
        { value: 'gamma', label: 'Gamma' },
      ],
      ...props,
    },
  })
  await wrapper.get('button').trigger('click')
  await nextTick()
}

async function search(query: string) {
  const input = document.querySelector<HTMLInputElement>('.select-search-input')!
  await new DOMWrapper(input).setValue(query)
}

async function enter() {
  document.activeElement!.dispatchEvent(new KeyboardEvent('keydown', { key: 'Enter', bubbles: true }))
  await nextTick()
}

describe('Select highlight after result changes', () => {
  it('selects a filtered result when the previous highlight index is no longer valid', async () => {
    await open()
    await search('alpha')
    await enter()
    expect(wrapper.emitted('update:modelValue')).toEqual([['alpha']])
  })

  it('restores keyboard selection after a search with no matches', async () => {
    await open()
    await search('no matches')
    await enter()
    expect(wrapper.emitted('update:modelValue')).toBeUndefined()
    await search('beta')
    await enter()
    expect(wrapper.emitted('update:modelValue')).toEqual([['beta']])
  })

  it('highlights the first enabled result when remote options arrive', async () => {
    await open({ modelValue: null, remote: true, options: [] })
    await wrapper.setProps({ options: [
      { value: 'group', label: 'Group', kind: 'group', disabled: true },
      { value: 'result', label: 'Result' },
    ] })
    await enter()
    expect(wrapper.emitted('update:modelValue')).toEqual([['result']])
  })

  it('does not retain an old index that now points to a disabled option', async () => {
    await open({ remote: true })
    await wrapper.setProps({ options: [
      { value: 'result', label: 'Result' },
      { value: 'other', label: 'Other' },
      { value: 'disabled', label: 'Disabled', disabled: true },
    ] })
    await enter()
    expect(wrapper.emitted('update:modelValue')).toEqual([['result']])
  })

  it('preserves the selected highlight when reopening after clearing a search', async () => {
    await open()
    await search('alpha')
    await wrapper.get('button').trigger('click')
    await wrapper.get('button').trigger('click')
    await nextTick()
    await enter()
    expect(wrapper.emitted('update:modelValue')).toEqual([['gamma']])
  })
})
