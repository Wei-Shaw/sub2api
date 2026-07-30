import { describe, expect, it, vi } from 'vitest'
import { mount } from '@vue/test-utils'
import PlazaModelCardGrid from '../PlazaModelCardGrid.vue'
import type { PlazaModel } from '@/api/modelPlaza'

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return {
    ...actual,
    useI18n: () => ({
      t: (key: string) => key
    })
  }
})

function tokenModel(overrides: Partial<PlazaModel> = {}): PlazaModel {
  return {
    name: 'gpt-test',
    platform: 'openai',
    pricing: {
      billing_mode: 'token',
      input_price: 3e-6,
      output_price: 1.5e-5,
      cache_write_price: 3.75e-6,
      cache_read_price: 3e-7,
      image_input_price: null,
      image_output_price: null,
      per_request_price: null,
      intervals: []
    },
    official_pricing: {
      input_price: 3e-6,
      output_price: 1.5e-5,
      cache_write_price: null,
      cache_read_price: null
    },
    ...overrides
  }
}

describe('PlazaModelCardGrid', () => {
  it('深色单色模型 Logo 使用主题前景色', () => {
    const wrapper = mount(PlazaModelCardGrid, {
      props: {
        models: [
          tokenModel(),
          tokenModel({ name: 'claude-test', platform: 'anthropic' })
        ],
        platform: 'openai',
        rateMultiplier: 1
      }
    })

    const cards = wrapper.findAll('[data-test="model-card"]')
    const openAILogo = cards.find((card) => card.text().includes('gpt-test'))!.get('.model-icon')
    const claudeLogo = cards.find((card) => card.text().includes('claude-test'))!.get('.model-icon')

    expect(openAILogo.classes()).toContain('dark:text-gray-100')
    expect(openAILogo.get('path').attributes('fill')).toBe('currentColor')
    expect(claudeLogo.get('path').attributes('fill')).toBe('#D97706')
  })

  it('按官方输出价降序渲染独立模型卡片', () => {
    const wrapper = mount(PlazaModelCardGrid, {
      props: {
        models: [
          tokenModel({ name: 'cheap', official_pricing: { input_price: 1e-6, output_price: 2e-6, cache_write_price: null, cache_read_price: null } }),
          tokenModel({ name: 'no-official', official_pricing: null }),
          tokenModel({ name: 'expensive', official_pricing: { input_price: 5e-6, output_price: 3e-5, cache_write_price: null, cache_read_price: null } })
        ],
        platform: 'openai',
        rateMultiplier: 1
      }
    })

    expect(wrapper.findAll('[data-test="model-card"]').map((card) => card.find('h3').text())).toEqual([
      'expensive',
      'cheap',
      'no-official'
    ])
  })

  it('实付价使用专属倍率，官方价保持原价并展示原倍率划线', () => {
    const wrapper = mount(PlazaModelCardGrid, {
      props: {
        models: [tokenModel()],
        platform: 'openai',
        rateMultiplier: 1,
        userRateMultiplier: 0.5
      }
    })

    const text = wrapper.text()
    expect(text).toContain('$1.50')
    expect(text).toContain('$7.50')
    expect(text).toContain('$3.00 / $15.00')
    expect(wrapper.find('.line-through').text()).toBe('1x')
    expect(text).toContain('0.5x')
  })

  it('按图阶梯定价逐档展示并使用图片单位', () => {
    const imageModel = tokenModel({
      name: 'gpt-image-test',
      pricing: {
        billing_mode: 'image',
        input_price: null,
        output_price: null,
        cache_write_price: null,
        cache_read_price: null,
        image_input_price: null,
        image_output_price: 3e-5,
        per_request_price: null,
        intervals: [
          {
            min_tokens: 0,
            max_tokens: null,
            tier_label: '1K',
            input_price: null,
            output_price: null,
            cache_write_price: null,
            cache_read_price: null,
            per_request_price: 0.01
          },
          {
            min_tokens: 0,
            max_tokens: null,
            tier_label: '2K',
            input_price: null,
            output_price: null,
            cache_write_price: null,
            cache_read_price: null,
            per_request_price: 0.02
          }
        ]
      },
      official_pricing: null
    })
    const wrapper = mount(PlazaModelCardGrid, {
      props: {
        models: [imageModel],
        platform: 'openai',
        rateMultiplier: 0.1
      }
    })

    const text = wrapper.text()
    expect(text).toContain('modelPlaza.table.perImage')
    expect(text).toContain('1K')
    expect(text).toContain('$0.001')
    expect(text).toContain('2K')
    expect(text).toContain('$0.002')
    expect(text).toContain('modelPlaza.table.perUnitImage')
    expect(text).not.toContain('$0.000003')
  })

  it('点击卡片时抛出所选模型', async () => {
    const model = tokenModel()
    const wrapper = mount(PlazaModelCardGrid, {
      props: {
        models: [model],
        platform: 'openai',
        rateMultiplier: 1
      }
    })

    await wrapper.get('[data-test="model-card"]').trigger('click')
    expect(wrapper.emitted('select')).toEqual([[model]])
  })
})
