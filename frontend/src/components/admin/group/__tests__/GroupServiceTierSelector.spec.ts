import { defineComponent, ref } from 'vue'
import { mount } from '@vue/test-utils'
import { describe, expect, it, vi } from 'vitest'
import type { OpenAIServiceTierPolicy } from '@/types'
import GroupServiceTierSelector from '../GroupServiceTierSelector.vue'

vi.mock('vue-i18n', () => ({ useI18n: () => ({ t: (key: string) => key }) }))

describe('GroupServiceTierSelector', () => {
  it('renders all modes and emits the selected tier', async () => {
    const wrapper = mount(GroupServiceTierSelector, { props: { modelValue: 'client' } })
    expect(wrapper.findAll('[role="radio"]')).toHaveLength(3)
    expect(wrapper.get('[data-tier="client"]').attributes('aria-checked')).toBe('true')
    await wrapper.get('[data-tier="ultrafast"]').trigger('click')
    await wrapper.get('[data-tier="priority"]').trigger('click')
    expect(wrapper.emitted('update:modelValue')).toEqual([['ultrafast'], ['priority']])
    wrapper.unmount()
  })

  it('supports keyboard navigation, wrapping and focus', async () => {
    const parent = defineComponent({
      components: { GroupServiceTierSelector },
      setup: () => ({ tier: ref<OpenAIServiceTierPolicy>('client') }),
      template: '<GroupServiceTierSelector v-model="tier" />',
    })
    const wrapper = mount(parent, { attachTo: document.body })
    await wrapper.get('[data-tier="client"]').trigger('keydown', { key: 'ArrowLeft' })
    expect(wrapper.get('[data-tier="ultrafast"]').attributes('aria-checked')).toBe('true')
    expect(document.activeElement).toBe(wrapper.get('[data-tier="ultrafast"]').element)
    await wrapper.get('[role="radiogroup"]').trigger('keydown', { key: 'Home' })
    expect(wrapper.get('[data-tier="client"]').attributes('aria-checked')).toBe('true')
    await wrapper.get('[role="radiogroup"]').trigger('keydown', { key: 'End' })
    expect(wrapper.get('[data-tier="ultrafast"]').attributes('aria-checked')).toBe('true')
    await wrapper.get('[role="radiogroup"]').trigger('keydown', { key: 'ArrowRight' })
    expect(wrapper.get('[data-tier="client"]').attributes('aria-checked')).toBe('true')
    wrapper.unmount()
  })

  it('disables every option while saving', async () => {
    const wrapper = mount(GroupServiceTierSelector, { props: { modelValue: 'priority', disabled: true } })
    for (const button of wrapper.findAll('button')) expect(button.attributes()).toHaveProperty('disabled')
    await wrapper.get('[data-tier="ultrafast"]').trigger('click')
    await wrapper.get('[role="radiogroup"]').trigger('keydown', { key: 'End' })
    expect(wrapper.emitted('update:modelValue')).toBeUndefined()
    wrapper.unmount()
  })
})
