import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { enableAutoUnmount, mount } from '@vue/test-utils'
import SearchInput from '../SearchInput.vue'

enableAutoUnmount(afterEach)
beforeEach(() => vi.useFakeTimers())
afterEach(() => vi.useRealTimers())
function mountInput() {
  return mount(SearchInput, { props: { modelValue: '', debounceMs: 300 }, global: { stubs: { Icon: true } } })
}

describe('SearchInput composition', () => {
  it('waits for committed IME text before updating or searching', async () => {
    const wrapper = mountInput()
    const input = wrapper.get('input')
    await input.trigger('compositionstart')
    input.element.value = 'zhong'
    await input.trigger('input')
    await vi.advanceTimersByTimeAsync(500)
    expect(wrapper.emitted('update:modelValue')).toBeUndefined()
    expect(wrapper.emitted('search')).toBeUndefined()
    input.element.value = '中文'
    await input.trigger('compositionend')
    expect(wrapper.emitted('update:modelValue')).toEqual([['中文']])
    await vi.advanceTimersByTimeAsync(300)
    expect(wrapper.emitted('search')).toEqual([['中文']])
  })

  it('still updates plain text immediately and debounces searches', async () => {
    const wrapper = mountInput()
    const input = wrapper.get('input')
    await input.setValue('a')
    await vi.advanceTimersByTimeAsync(200)
    await input.setValue('ab')
    expect(wrapper.emitted('update:modelValue')).toEqual([['a'], ['ab']])
    await vi.advanceTimersByTimeAsync(299)
    expect(wrapper.emitted('search')).toBeUndefined()
    await vi.advanceTimersByTimeAsync(1)
    expect(wrapper.emitted('search')).toEqual([['ab']])
  })

  it('reflects external value changes without issuing a search', async () => {
    const wrapper = mountInput()
    await wrapper.setProps({ modelValue: 'restored' })
    expect(wrapper.get('input').element.value).toBe('restored')
    await vi.advanceTimersByTimeAsync(500)
    expect(wrapper.emitted('search')).toBeUndefined()
  })
})
