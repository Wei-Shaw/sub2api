import { describe, it, expect } from 'vitest'
import { mount } from '@vue/test-utils'
import GroupOptionItem from '../GroupOptionItem.vue'
import { createI18n } from 'vue-i18n'

const i18n = createI18n({
  legacy: false,
  locale: 'zh',
  messages: {
    zh: {
      common: {
        groupOption: {
          // Use MessageFunction to avoid runtime compiler requirement
          discountOriginal: () => '原价',
          discountFormat: ({ named }: { named: (key: string) => unknown }) =>
            `${named('value')}折`,
          cacheFormat: ({ named }: { named: (key: string) => unknown }) =>
            `缓存 ${named('pct')}%`
        }
      }
    }
  }
})

function mountItem(props: Record<string, unknown> = {}) {
  return mount(GroupOptionItem, {
    props: {
      name: 'test-group',
      platform: 'anthropic',
      rateMultiplier: 4,
      ...props
    },
    global: { plugins: [i18n] }
  })
}

describe('GroupOptionItem discount pill', () => {
  it('renders "5.7折" for rate 4 (4/7×10 = 5.71)', () => {
    const wrapper = mountItem({ rateMultiplier: 4 })
    expect(wrapper.text()).toContain('5.7折')
  })

  it('renders "原价" for rate 7', () => {
    const wrapper = mountItem({ rateMultiplier: 7 })
    expect(wrapper.text()).toContain('原价')
  })

  it('renders "1.4折" for rate 1', () => {
    const wrapper = mountItem({ rateMultiplier: 1 })
    expect(wrapper.text()).toContain('1.4折')
  })

  it('omits discount pill when rate is 0', () => {
    const wrapper = mountItem({ rateMultiplier: 0 })
    expect(wrapper.text()).not.toContain('折')
    expect(wrapper.text()).not.toContain('原价')
  })

  it('uses userRateMultiplier when present', () => {
    const wrapper = mountItem({ rateMultiplier: 4, userRateMultiplier: 1 })
    expect(wrapper.text()).toContain('1.4折')
  })
})

describe('GroupOptionItem cache pill', () => {
  it('renders "缓存 35%" for cacheHitRate7d=0.347', () => {
    const wrapper = mountItem({ cacheHitRate7d: 0.347 })
    expect(wrapper.text()).toContain('缓存 35%')
  })

  it('renders "缓存 0%" for cacheHitRate7d=0 (zero, not null)', () => {
    const wrapper = mountItem({ cacheHitRate7d: 0 })
    expect(wrapper.text()).toContain('缓存 0%')
  })

  it('omits cache pill when cacheHitRate7d is null', () => {
    const wrapper = mountItem({ cacheHitRate7d: null })
    expect(wrapper.text()).not.toContain('缓存')
  })

  it('omits cache pill when cacheHitRate7d is undefined', () => {
    const wrapper = mountItem({})
    expect(wrapper.text()).not.toContain('缓存')
  })
})
