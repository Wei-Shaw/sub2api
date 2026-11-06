import { defineComponent, ref } from 'vue'
import { mount } from '@vue/test-utils'
import { describe, expect, it, vi } from 'vitest'
import ImagePriceExpressionsField from '../ImagePriceExpressionsField.vue'
import { grokImageExpressionModel, grokImageExpressionPreset, type ImagePriceExpressionRow } from '../imagePriceExpressions'

vi.mock('vue-i18n', () => ({ useI18n: () => ({ t: (key: string) => key }) }))

function mountEditor(initial: ImagePriceExpressionRow[] = []) {
  return mount(defineComponent({
    components: { ImagePriceExpressionsField },
    setup: () => ({ rows: ref(initial) }),
    template: '<ImagePriceExpressionsField v-model="rows" />'
  }))
}

describe('ImagePriceExpressionsField', () => {
  it('adds the Grok example once, preserves existing rules and uses explicit image count', async () => {
    const wrapper = mountEditor([{ model: 'custom', expression: 'fixed(0.15)' }])
    await wrapper.get('[data-testid="image-expression-preset"]').trigger('click')
    const rows = wrapper.findAll('[data-testid="image-expression-row"]')
    expect(rows).toHaveLength(2)
    expect(rows[0].get<HTMLTextAreaElement>('textarea').element.value).toBe('fixed(0.15)')
    expect(rows[1].get<HTMLInputElement>('input').element.value).toBe(grokImageExpressionModel)
    expect(rows[1].get<HTMLTextAreaElement>('textarea').element.value).toBe(grokImageExpressionPreset)
    expect(grokImageExpressionPreset).toContain('* image_count')
    expect(wrapper.get('[data-testid="image-expression-preset"]').attributes('disabled')).toBeDefined()
    wrapper.unmount()
  })

  it('shows incomplete and duplicate errors while retaining editable rows', async () => {
    const wrapper = mountEditor([{ model: 'custom', expression: 'fixed(0.15)' }])
    await wrapper.get('[data-testid="image-expression-add"]').trigger('click')
    const models = wrapper.findAll('[data-testid="image-expression-model"]')
    await models[1].setValue('custom')
    expect(wrapper.get('[role="alert"]').text()).toContain('incompleteRow')
    await wrapper.findAll('[data-testid="image-expression-source"]')[1].setValue('fixed(0.2)')
    expect(wrapper.get('[role="alert"]').text()).toContain('duplicateModel')
    await models[1].setValue('Custom')
    expect(wrapper.find('[role="alert"]').exists()).toBe(false)
    await wrapper.findAll('[data-testid="image-expression-remove"]')[0].trigger('click')
    await wrapper.get('[data-testid="image-expression-remove"]').trigger('click')
    expect(wrapper.findAll('[data-testid="image-expression-row"]')).toHaveLength(0)
    expect(wrapper.findComponent(ImagePriceExpressionsField).props('modelValue')).toEqual([])
    wrapper.unmount()
  })
})
