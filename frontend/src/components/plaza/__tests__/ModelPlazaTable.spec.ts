import { mount } from '@vue/test-utils'
import { describe, expect, it, vi } from 'vitest'
import { nextTick } from 'vue'
import ModelPlazaTable from '../ModelPlazaTable.vue'
import Select from '@/components/common/Select.vue'
import type { PlazaModelRow } from '@/api/plaza'

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return {
    ...actual,
    useI18n: () => ({ t: (key: string) => key }),
  }
})

function tokenRow(over: Partial<PlazaModelRow> = {}): PlazaModelRow {
  return {
    group_id: 1,
    group_name: 'Default',
    platform: 'claude',
    model: 'claude-3-5-sonnet',
    type: 'token',
    input_price_per_mtok: 3,
    output_price_per_mtok: 15,
    site_input_price_per_mtok: 3,
    site_output_price_per_mtok: 15,
    multiplier: 1,
    discount_percent: 0,
    ...over,
  }
}

function imageRow(over: Partial<PlazaModelRow> = {}): PlazaModelRow {
  return {
    group_id: 2,
    group_name: 'Image',
    platform: 'openai',
    model: 'dall-e-3',
    type: 'image',
    multiplier: 1,
    discount_percent: 0,
    base_image_prices: { tier_1k: 0.04, tier_2k: 0.08, tier_4k: 0.12 },
    site_image_prices: { tier_1k: 0.04, tier_2k: 0.08, tier_4k: 0.12 },
    ...over,
  }
}

const formatUsd = (amount: number, digits = 4) => `$${amount.toFixed(digits)}`

function mountTable(
  rows: PlazaModelRow[],
  opts: {
    loading?: boolean
    selectedGroupId?: number
    platformOptions?: string[]
    formatBase?: (amount: number, digits?: number) => string
    rechargeMultiplier?: number | null
  } = {}
) {
  return mount(ModelPlazaTable, {
    props: {
      rows,
      loading: opts.loading ?? false,
      currencyDisplay: 'USD' as const,
      formatUsd,
      formatBase: opts.formatBase,
      rechargeMultiplier: opts.rechargeMultiplier,
      selectedGroupId: opts.selectedGroupId,
      platformOptions: opts.platformOptions,
    },
  })
}

describe('ModelPlazaTable', () => {
  it('渲染 token 行的输入/输出双价格 + Mtok 后缀', () => {
    const wrapper = mountTable([tokenRow()])
    const text = wrapper.text()
    expect(text).toContain('claude-3-5-sonnet')
    // 后缀已从紧凑形式 "/Mtok" 升级为可读的 " / M Tokens"（见
    // ModelPlazaTable.vue PRICE_UNIT_SUFFIX）。这里跟着改成完整后缀，
    // 确保未来若 PRICE_UNIT_SUFFIX 再变（例如本地化 / 单复数），
    // 能被这条测试第一时间发现。
    expect(text).toContain('$3.0000 / M Tokens')
    expect(text).toContain('$15.0000 / M Tokens')
  })

  it('渲染 image 行的三档价格', () => {
    const wrapper = mountTable([imageRow()])
    const text = wrapper.text()
    expect(text).toContain('dall-e-3')
    expect(text).toContain('1K')
    expect(text).toContain('2K')
    expect(text).toContain('4K')
    expect(text).toContain('$0.0400')
    expect(text).toContain('$0.0800')
    expect(text).toContain('$0.1200')
  })

  it('搜索框过滤模型名（不区分大小写）', async () => {
    const wrapper = mountTable([
      tokenRow({ model: 'claude-3-5-sonnet' }),
      tokenRow({ model: 'gpt-4o', platform: 'openai' }),
    ])
    expect(wrapper.text()).toContain('claude-3-5-sonnet')
    expect(wrapper.text()).toContain('gpt-4o')

    const input = wrapper.find('input[type="text"]')
    await input.setValue('CLAUDE')
    await nextTick()
    expect(wrapper.text()).toContain('claude-3-5-sonnet')
    expect(wrapper.text()).not.toContain('gpt-4o')
  })

  it('selectedGroupId 受控：精确匹配过滤', async () => {
    const wrapper = mountTable(
      [
        tokenRow({ group_id: 1, group_name: 'A', model: 'm-a' }),
        tokenRow({ group_id: 2, group_name: 'B', model: 'm-b' }),
      ],
      { selectedGroupId: undefined }
    )
    expect(wrapper.text()).toContain('m-a')
    expect(wrapper.text()).toContain('m-b')

    await wrapper.setProps({ selectedGroupId: 2 })
    await nextTick()
    expect(wrapper.text()).not.toContain('m-a')
    expect(wrapper.text()).toContain('m-b')
  })

  it('platform 下拉用公共 Select 组件，过滤精确匹配', async () => {
    const wrapper = mountTable([
      tokenRow({ model: 'm-claude', platform: 'claude' }),
      tokenRow({ model: 'm-gpt', platform: 'openai', group_id: 9, group_name: 'G' }),
    ])
    const selects = wrapper.findAllComponents(Select)
    expect(selects.length).toBe(1) // group 已下放到父组件，仅剩 platform
    selects[0].vm.$emit('update:modelValue', 'openai')
    await nextTick()
    expect(wrapper.text()).not.toContain('m-claude')
    expect(wrapper.text()).toContain('m-gpt')
  })

  it('点击 Reset 后 q + platform 过滤恢复', async () => {
    const wrapper = mountTable([
      tokenRow({ model: 'm-claude', platform: 'claude' }),
      tokenRow({ model: 'm-gpt', platform: 'openai', group_id: 9, group_name: 'G' }),
    ])
    const input = wrapper.find('input[type="text"]')
    await input.setValue('claude')
    await nextTick()
    expect(wrapper.text()).not.toContain('m-gpt')

    // mock 下 t('plaza.common.reset') 直接返回 key，按文本匹配 reset 按钮
    const buttons = wrapper.findAll('button')
    const resetBtn = buttons.find((b) => b.text().includes('plaza.common.reset'))
    expect(resetBtn).toBeDefined()
    await resetBtn!.trigger('click')
    await nextTick()
    expect(wrapper.text()).toContain('m-claude')
    expect(wrapper.text()).toContain('m-gpt')
  })

  it('rows 为空显示空态 key', () => {
    const wrapper = mountTable([])
    expect(wrapper.text()).toContain('plaza.models.empty')
  })

  it('loading 时显示 loading key', () => {
    const wrapper = mountTable([], { loading: true })
    expect(wrapper.text()).toContain('common.loading')
  })

  it('搜索框 maxlength 限制为 64', () => {
    const wrapper = mountTable([tokenRow()])
    const input = wrapper.find('input[type="text"]')
    expect(input.attributes('maxlength')).toBe('64')
  })

  it('platformOptions 优先于 rows 派生，保持稳定', () => {
    const wrapper = mountTable([tokenRow({ platform: 'claude' })], {
      platformOptions: ['claude', 'openai', 'gemini'],
    })
    const select = wrapper.findAllComponents(Select)[0]
    const opts = (select.props('options') as Array<{ value: string; label: string }>)
    // 第一项是 "All platforms" sentinel；其后应包含外部传入的全部 3 个选项
    expect(opts[0].value).toBe('')
    expect(opts.slice(1).map((o) => o.value)).toEqual(['claude', 'openai', 'gemini'])
  })

  it('formatBase 优先于 formatUsd 渲染原价（即原价不随币种切换变化）', () => {
    // 模拟：当前币种 = CNY 时 formatUsd 会换算成 ¥，而 formatBase 始终保持 USD
    const formatBase = (amount: number, digits = 4) => `$${amount.toFixed(digits)}`
    const formatUsdAsCny = (amount: number, digits = 4) => `¥${(amount * 7).toFixed(digits)}`
    const wrapper = mount(ModelPlazaTable, {
      props: {
        rows: [tokenRow({ input_price_per_mtok: 3, output_price_per_mtok: 15 })],
        loading: false,
        currencyDisplay: 'CNY' as const,
        formatUsd: formatUsdAsCny,
        formatBase,
        platformOptions: [],
      },
    })
    const text = wrapper.text()
    // 原价（base）始终是 USD；后缀同上，跟随组件 PRICE_UNIT_SUFFIX。
    expect(text).toContain('$3.0000 / M Tokens')
    expect(text).toContain('$15.0000 / M Tokens')
    // 站点价（site）走 formatUsd → 显示为 ¥
    expect(text).toContain('¥21.0000 / M Tokens')
    expect(text).toContain('¥105.0000 / M Tokens')
  })

  it('rechargeMultiplier > 0 时渲染充值比例横幅', () => {
    const wrapper = mountTable([tokenRow()], { rechargeMultiplier: 0.14 })
    const text = wrapper.text()
    expect(text).toContain('plaza.models.rechargeRatio')
    expect(text).toContain('plaza.models.rechargeNote')
    expect(text).toContain('plaza.models.rechargeHint')
  })

  it('rechargeMultiplier 为 0 / null 时不渲染横幅', () => {
    const wA = mountTable([tokenRow()], { rechargeMultiplier: 0 })
    expect(wA.text()).not.toContain('plaza.models.rechargeRatio')
    const wB = mountTable([tokenRow()], { rechargeMultiplier: null })
    expect(wB.text()).not.toContain('plaza.models.rechargeRatio')
  })
})
