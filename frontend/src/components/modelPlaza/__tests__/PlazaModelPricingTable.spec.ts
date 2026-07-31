import { describe, expect, it, vi } from 'vitest'
import { mount } from '@vue/test-utils'
import PlazaModelPricingTable from '../PlazaModelPricingTable.vue'
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
    name: 'claude-sonnet',
    platform: 'anthropic',
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
      cache_write_price: 3.75e-6,
      cache_write_1h_price: 6e-6,
      cache_read_price: 3e-7
    },
    ...overrides
  }
}

function mountTable(models: PlazaModel[], rateMultiplier: number, userRateMultiplier?: number | null) {
  return mount(PlazaModelPricingTable, {
    props: { models, rateMultiplier, userRateMultiplier: userRateMultiplier ?? null }
  })
}

describe('PlazaModelPricingTable', () => {
  it('倍率为 1 时展示渠道单价原值($/1M),价格保底 2 位小数', () => {
    const wrapper = mountTable([tokenModel()], 1)
    const text = wrapper.text()
    expect(text).toContain('$3.00')
    expect(text).toContain('$15.00')
    // 缓存写 / 读(超过 2 位小数原样保留)
    expect(text).toContain('$3.75')
    expect(text).toContain('$0.30')
    // 倍率列
    expect(text).toContain('1x')
  })

  it('倍率 ≠ 1 时价格列为折后实付价,官方价列保持原价', () => {
    const wrapper = mountTable([tokenModel()], 0.5)
    const text = wrapper.text()
    // 实付 = 3 × 0.5 / 15 × 0.5
    expect(text).toContain('$1.50')
    expect(text).toContain('$7.50')
    // 官方价原值仍在(官方列不乘倍率)
    expect(text).toContain('$3.00')
    expect(text).toContain('$15.00')
    expect(text).toContain('0.5x')
  })

  it('用户专属倍率覆盖分组倍率,并划线展示原倍率', () => {
    const wrapper = mountTable([tokenModel()], 1, 0.8)
    const text = wrapper.text()
    // 实付按 0.8:3 × 0.8 = 2.4
    expect(text).toContain('$2.40')
    expect(text).toContain('$12.00')
    // 倍率列:原倍率划线 + 专属倍率
    const struck = wrapper.find('td .line-through')
    expect(struck.exists()).toBe(true)
    expect(struck.text()).toBe('1x')
    expect(text).toContain('0.8x')
  })

  it('模型按官方输出价从高到低排序,无官方价的排最后', () => {
    const expensive = tokenModel({
      name: 'model-expensive',
      official_pricing: {
        input_price: 1e-5,
        output_price: 7.5e-5,
        cache_write_price: null,
        cache_write_1h_price: null,
        cache_read_price: null
      }
    })
    const cheap = tokenModel({
      name: 'model-cheap',
      official_pricing: {
        input_price: 1e-6,
        output_price: 5e-6,
        cache_write_price: null,
        cache_write_1h_price: null,
        cache_read_price: null
      }
    })
    const noOfficial = tokenModel({ name: 'model-no-official', official_pricing: null })

    const wrapper = mountTable([cheap, noOfficial, expensive], 1)
    const names = wrapper.findAll('tbody tr').map((tr) => tr.find('td').text())
    expect(names).toEqual(['model-expensive', 'model-cheap', 'model-no-official'])
  })

  it('两级表头:实付区与官方区各拆输入/输出/缓存列', () => {
    const wrapper = mountTable([tokenModel()], 1)
    const text = wrapper.text()
    expect(text).toContain('modelPlaza.table.paidPrice')
    expect(text).toContain('modelPlaza.table.officialPrice')
    // token 行:模型 + 实付 3 列 + 官方 3 列 + 倍率
    expect(wrapper.findAll('tbody td')).toHaveLength(8)
  })

  it('官方价包含 1h 缓存写入价;official_pricing 为 null 时官方三列显示 -', () => {
    const withOfficial = mountTable([tokenModel()], 1)
    expect(withOfficial.text()).toContain('$6.00')
    expect(withOfficial.text()).toContain('(1h')

    const withoutOfficial = mountTable([tokenModel({ official_pricing: null })], 1)
    const cells = withoutOfficial.findAll('tbody td')
    // 官方 输入/输出/缓存 三列均为 -
    expect(cells[4].text().trim()).toBe('-')
    expect(cells[5].text().trim()).toBe('-')
    expect(cells[6].text().trim()).toBe('-')
  })

  it('per_request 模型按单次价 × 倍率展示,官方价列显示 -', () => {
    const model = tokenModel({
      name: 'search-tool',
      pricing: {
        billing_mode: 'per_request',
        input_price: null,
        output_price: null,
        cache_write_price: null,
        cache_read_price: null,
        image_input_price: null,
        image_output_price: null,
        per_request_price: 0.04,
        intervals: []
      },
      official_pricing: null
    })
    const wrapper = mountTable([model], 0.5)
    const text = wrapper.text()
    // 0.04 × 0.5 = 0.02,scale=1
    expect(text).toContain('$0.02')
    expect(text).toContain('modelPlaza.table.perRequest')
    // 单位后缀跟在价格后(按次 → / 次)
    expect(text).toContain('modelPlaza.table.perUnitRequest')
  })

  it('token 模型阶梯定价内联进输入/输出列,按倍率折算', () => {
    const model = tokenModel({
      pricing: {
        billing_mode: 'token',
        input_price: 3e-6,
        output_price: 1.5e-5,
        cache_write_price: null,
        cache_read_price: null,
        image_input_price: null,
        image_output_price: null,
        per_request_price: null,
        intervals: [
          {
            min_tokens: 0,
            max_tokens: 200000,
            tier_label: '',
            input_price: 3e-6,
            output_price: 1.5e-5,
            cache_write_price: null,
            cache_read_price: null,
            per_request_price: null
          },
          {
            min_tokens: 200000,
            max_tokens: null,
            tier_label: '',
            input_price: 6e-6,
            output_price: 3e-5,
            cache_write_price: null,
            cache_read_price: null,
            per_request_price: null
          }
        ]
      }
    })
    const wrapper = mountTable([model], 0.5)
    const text = wrapper.text()
    // 区间标签按 token 数生成
    expect(text).toContain('≤200K')
    expect(text).toContain('>200K')
    // 折后:输入 1.5 / 3,输出 7.5 / 15
    expect(text).toContain('$1.50')
    expect(text).toContain('$7.50')
    expect(text).toContain('$15.00')
  })

  it('按图模型主行展示阶梯芯片,不把 image_output_price(每 token)当按次价', () => {
    const model = tokenModel({
      name: 'gpt-image-2',
      pricing: {
        billing_mode: 'image',
        input_price: null,
        output_price: null,
        cache_write_price: null,
        cache_read_price: null,
        image_input_price: null,
        // 每 token 图片输出价:不应被当作按次单价展示
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
    const wrapper = mountTable([model], 0.1)
    const text = wrapper.text()
    expect(text).toContain('modelPlaza.table.perImage')
    // 芯片:1K $0.001 / 2K $0.002,单位后缀内嵌(按图 → / 张)
    expect(text).toContain('1K')
    expect(text).toContain('$0.001')
    expect(text).toContain('2K')
    expect(text).toContain('$0.002')
    expect(text).toContain('modelPlaza.table.perUnitImage')
    // 旧 bug:image_output_price × 0.1 = 0.000003 被当按次价
    expect(text).not.toContain('$0.000003')
  })
})
