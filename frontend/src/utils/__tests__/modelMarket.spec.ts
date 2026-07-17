import { describe, expect, it } from 'vitest'

import { buildModelMarketItems, filterModelMarketItems, getModelMarketFilterOptions } from '@/utils/modelMarket'
import type { UserAvailableChannel } from '@/api/channels'

const channels: UserAvailableChannel[] = [
  {
    name: '标准渠道',
    description: '公开可用',
    platforms: [
      {
        platform: 'openai',
        groups: [
          {
            id: 2,
            name: 'OpenAI Pro',
            platform: 'openai',
            subscription_type: 'standard',
            rate_multiplier: 1.2,
            is_exclusive: false
          }
        ],
        supported_models: [
          {
            name: 'gpt-4o-mini',
            platform: 'openai',
            pricing: {
              billing_mode: 'token',
              input_price: 0.000001,
              output_price: 0.000002,
              cache_write_price: null,
              cache_read_price: null,
              image_output_price: null,
              per_request_price: null,
              intervals: []
            }
          },
          {
            name: 'gpt-image-2',
            platform: 'openai',
            pricing: null
          }
        ]
      }
    ]
  },
  {
    name: '专属渠道',
    description: '定向授权',
    platforms: [
      {
        platform: 'openai',
        groups: [
          {
            id: 3,
            name: 'OpenAI VIP',
            platform: 'openai',
            subscription_type: 'subscription',
            rate_multiplier: 0.8,
            is_exclusive: true
          }
        ],
        supported_models: [
          {
            name: 'GPT-4O-MINI',
            platform: 'openai',
            pricing: null
          }
        ]
      },
      {
        platform: 'anthropic',
        groups: [
          {
            id: 1,
            name: 'Claude',
            platform: 'anthropic',
            subscription_type: 'standard',
            rate_multiplier: 1,
            is_exclusive: false
          }
        ],
        supported_models: [
          {
            name: 'claude-sonnet-4-5',
            platform: 'anthropic',
            pricing: null
          }
        ]
      }
    ]
  }
]

describe('modelMarket', () => {
  it('按分组聚合渠道与模型，不把同一模型跨分组合并成一行', () => {
    const items = buildModelMarketItems(channels)

    expect(items.map((item) => item.group.name)).toEqual(['Claude', 'OpenAI VIP', 'OpenAI Pro'])

    const pro = items.find((item) => item.group.name === 'OpenAI Pro')
    const vip = items.find((item) => item.group.name === 'OpenAI VIP')

    expect(pro).toMatchObject({
      platform: 'openai',
      channel_count: 1,
      model_count: 2,
      has_pricing: true
    })
    expect(pro?.channels.map((ch) => ch.name)).toEqual(['标准渠道'])
    expect(pro?.models.map((model) => model.name)).toEqual(['gpt-4o-mini', 'gpt-image-2'])

    expect(vip).toMatchObject({
      platform: 'openai',
      channel_count: 1,
      model_count: 1,
      has_pricing: false
    })
    expect(vip?.channels.map((ch) => ch.name)).toEqual(['专属渠道'])
    expect(vip?.models.map((model) => model.name)).toEqual(['GPT-4O-MINI'])
  })

  it('同一分组跨渠道同模型不同定价时只保留定价状态，避免展示任意单一价格', () => {
    const items = buildModelMarketItems([
      channels[0],
      {
        name: '备用渠道',
        description: '不同价格',
        platforms: [
          {
            platform: 'openai',
            groups: channels[0].platforms[0].groups,
            supported_models: [
              {
                name: 'gpt-4o-mini',
                platform: 'openai',
                pricing: {
                  billing_mode: 'token',
                  input_price: 0.000009,
                  output_price: 0.000012,
                  cache_write_price: null,
                  cache_read_price: null,
                  image_output_price: null,
                  per_request_price: null,
                  intervals: []
                }
              }
            ]
          }
        ]
      }
    ])

    const pro = items.find((item) => item.group.name === 'OpenAI Pro')
    const model = pro?.models.find((m) => m.name === 'gpt-4o-mini')

    expect(pro).toMatchObject({
      channel_count: 2,
      model_count: 2,
      has_pricing: true
    })
    expect(model).toMatchObject({
      name: 'gpt-4o-mini',
      platform: 'openai',
      pricing: null,
      has_pricing: true,
      pricing_conflict: true
    })

    expect(filterModelMarketItems(items, { search: '', platform: '', channel: '', pricing: 'with' })[0]?.models.map((m) => m.name)).toEqual([
      'gpt-4o-mini'
    ])
  })

  it('同一分组跨渠道模型集合不同，按渠道过滤时只保留该渠道模型', () => {
    const sharedGroup = channels[0].platforms[0].groups[0]
    const items = buildModelMarketItems([
      {
        name: '渠道 A',
        description: '只支持 A 模型',
        platforms: [
          {
            platform: 'openai',
            groups: [sharedGroup],
            supported_models: [
              {
                name: 'model-a',
                platform: 'openai',
                pricing: null
              }
            ]
          }
        ]
      },
      {
        name: '渠道 B',
        description: '只支持 B 模型',
        platforms: [
          {
            platform: 'openai',
            groups: [sharedGroup],
            supported_models: [
              {
                name: 'model-b',
                platform: 'openai',
                pricing: null
              }
            ]
          }
        ]
      }
    ])

    const filtered = filterModelMarketItems(items, { search: '', platform: '', channel: '渠道 A', pricing: 'all' })

    expect(filtered).toHaveLength(1)
    expect(filtered[0]?.channels.map((channel) => channel.name)).toEqual(['渠道 A'])
    expect(filtered[0]?.models.map((model) => model.name)).toEqual(['model-a'])
  })

  it('生成稳定的筛选项', () => {
    const items = buildModelMarketItems(channels)
    const options = getModelMarketFilterOptions(items)

    expect(options.platforms).toEqual(['anthropic', 'openai'])
    expect(options.channels).toEqual(['标准渠道', '专属渠道'])
  })

  it('按搜索、平台、渠道和定价状态过滤分组，并裁剪不匹配的模型', () => {
    const items = buildModelMarketItems(channels)

    expect(filterModelMarketItems(items, { search: 'sonnet', platform: '', channel: '', pricing: 'all' }).map((item) => item.group.name)).toEqual(['Claude'])
    expect(filterModelMarketItems(items, { search: '', platform: 'anthropic', channel: '', pricing: 'all' }).map((item) => item.group.name)).toEqual(['Claude'])
    expect(filterModelMarketItems(items, { search: '', platform: '', channel: '专属渠道', pricing: 'all' }).map((item) => item.group.name)).toEqual(['Claude', 'OpenAI VIP'])

    const withPricing = filterModelMarketItems(items, { search: '', platform: '', channel: '', pricing: 'with' })
    expect(withPricing.map((item) => item.group.name)).toEqual(['OpenAI Pro'])
    expect(withPricing[0]?.models.map((model) => model.name)).toEqual(['gpt-4o-mini'])

    const withoutPricing = filterModelMarketItems(items, { search: '', platform: '', channel: '', pricing: 'without' })
    expect(withoutPricing.map((item) => item.group.name)).toEqual(['Claude', 'OpenAI VIP', 'OpenAI Pro'])
    expect(withoutPricing.find((item) => item.group.name === 'OpenAI Pro')?.models.map((model) => model.name)).toEqual(['gpt-image-2'])
  })
})
