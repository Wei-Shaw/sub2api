import { mount } from '@vue/test-utils'
import { describe, expect, it } from 'vitest'

import { PROVIDER_ANTHROPIC, PROVIDER_KIRO } from '@/constants/channelMonitor'
import ProviderIcon from '../monitor/ProviderIcon.vue'

describe('ProviderIcon', () => {
  // kiro 与其它 provider 一样走 currentColor 单色，但保留官方图形：原稿是 1200
  // 画板，且双眼必须与主体同属一条 path（靠 fill-rule="evenodd" 镂空），
  // 拆成独立 path 在单色下会盖住主体，图形就糊了。
  it('renders the Kiro glyph monochrome on its native 1200 viewBox', () => {
    const wrapper = mount(ProviderIcon, { props: { provider: PROVIDER_KIRO, size: 18 } })

    const svg = wrapper.get('svg')
    expect(svg.attributes('viewBox')).toBe('0 0 1200 1200')
    expect(svg.attributes('width')).toBe('18')
    expect(svg.attributes('fill')).toBe('currentColor')
    expect(svg.attributes('fill-rule')).toBe('evenodd')

    // 单色：不得残留品牌底板或硬编码填充色。
    expect(wrapper.find('rect').exists()).toBe(false)
    expect(svg.html()).not.toContain('#9046FF')
    expect(svg.html()).not.toContain('fill="white"')
    expect(svg.html()).not.toContain('fill="black"')

    // 主体 + 双眼合并为一条 path：三个子路径起点，双眼才能被挖空。
    const paths = wrapper.findAll('path')
    expect(paths).toHaveLength(1)
    expect(paths[0].attributes('d')?.match(/M/g) ?? []).toHaveLength(3)

    expect(wrapper.find('span').exists()).toBe(false)
  })

  it('renders monochrome currentColor icons for catalog providers', () => {
    const wrapper = mount(ProviderIcon, { props: { provider: PROVIDER_ANTHROPIC } })

    const svg = wrapper.get('svg')
    expect(svg.attributes('viewBox')).toBe('0 0 24 24')
    expect(svg.attributes('fill')).toBe('currentColor')
    expect(wrapper.findAll('path').length).toBeGreaterThan(0)
  })

  it('falls back to the first letter for unknown providers', () => {
    const wrapper = mount(ProviderIcon, { props: { provider: 'somethingnew' } })

    expect(wrapper.find('svg').exists()).toBe(false)
    expect(wrapper.get('span').text()).toBe('S')
  })
})
