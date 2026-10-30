import { mount } from '@vue/test-utils'
import { describe, expect, it, vi } from 'vitest'
import MiniMaxGroupPricing from '../MiniMaxGroupPricing.vue'

vi.mock('vue-i18n', () => ({ useI18n: () => ({ t: (key: string) => key }) }))

describe('MiniMax group price overrides', () => {
  it('preserves free prices and restores the default when cleared', async () => {
    const wrapper = mount(MiniMaxGroupPricing, { props: { modelValue: {}, independent: false, multiplier: 7, groupMultiplier: 2 } })
    const price = wrapper.get('input[aria-label="image-01 image"]')
    expect(price.attributes('placeholder')).toBe('0.0035')
    expect(wrapper.text()).toContain('$0.007')
    await price.setValue('0')
    expect(wrapper.emitted('update:modelValue')?.at(-1)).toEqual([{ 'image-01': { image: 0 } }])
    await wrapper.setProps({ modelValue: { 'image-01': { image: 0 } } })
    await price.setValue('')
    expect(wrapper.emitted('update:modelValue')?.at(-1)).toEqual([{}])
    await wrapper.setProps({ independent: true, multiplier: 3, modelValue: {} })
    expect(wrapper.text()).toContain('$0.0105')
  })
  it('previews legacy retail overrides with the common group multiplier', () => {
    const wrapper = mount(MiniMaxGroupPricing, { props: {
      modelValue: {}, independent: false, multiplier: 99, groupMultiplier: 2,
      legacy: [{ models: ['speech-2.8-hd'], minimax_media: { retail: { characters_per_million: 350 }, cost: {}, cost_currency: 'CNY', cny_per_usd: 1, multiplier: 99 } }],
    } })
    expect(wrapper.get('input[aria-label="speech-2.8-hd characters_per_million"]').attributes('placeholder')).toBe('350')
    expect(wrapper.text()).toContain('$700')
  })
  it('uses the last valid legacy price and ignores ordinary model cards', () => {
    const card = (price: number) => ({
      models: ['image-01'],
      minimax_media: { retail: { image: price }, cost: {}, cost_currency: 'USD' as const, cny_per_usd: 1, multiplier: 1 },
    })
    const wrapper = mount(MiniMaxGroupPricing, { props: {
      modelValue: {}, independent: false, multiplier: 1, groupMultiplier: 2,
      legacy: [{ models: ['image-01'], minimax_media: null }, card(.1), card(.2), card(1e7)],
    } })
    expect(wrapper.get('input[aria-label="image-01 image"]').attributes('placeholder')).toBe('0.2')
    expect(wrapper.text()).toContain('$0.4')
  })

  it('lets a MiniMax group enable and disable its image/video permission', async () => {
    const wrapper = mount(MiniMaxGroupPricing, { props: {
      modelValue: {}, independent: false, multiplier: 1, groupMultiplier: 1,
      showGenerationPermission: true, allowGeneration: false,
    } })
    const permission = wrapper.get<HTMLInputElement>('[data-testid="minimax-generation-permission"]')
    expect(permission.element.checked).toBe(false)
    await permission.setValue(true)
    expect(wrapper.emitted('update:allowGeneration')?.at(-1)).toEqual([true])
    await wrapper.setProps({ allowGeneration: true })
    expect(permission.element.checked).toBe(true)
    await permission.setValue(false)
    expect(wrapper.emitted('update:allowGeneration')?.at(-1)).toEqual([false])
  })

  it('leaves the shared image permission control to composite group settings', () => {
    const wrapper = mount(MiniMaxGroupPricing, { props: {
      modelValue: {}, independent: false, multiplier: 1, groupMultiplier: 1,
      showGenerationPermission: false, allowGeneration: true,
    } })
    expect(wrapper.find('[data-testid="minimax-generation-permission"]').exists()).toBe(false)
  })

})
