import { mount } from '@vue/test-utils'
import { describe, expect, it } from 'vitest'
import PlatformIcon from '../PlatformIcon.vue'

function getSvgViewBox(wrapper: ReturnType<typeof mount>): string {
  const attrs = wrapper.get('svg').attributes()
  return attrs.viewBox || attrs.viewbox || ''
}

describe('PlatformIcon', () => {
  it('Kiro 使用独立官方图标，不复用 Anthropic 图标', () => {
    const wrapper = mount(PlatformIcon, {
      props: {
        platform: 'kiro'
      }
    })

    expect(getSvgViewBox(wrapper)).toBe('0 0 1200 1200')
    expect(wrapper.html()).toContain('#9046FF')
    expect(wrapper.html()).toContain('M398.554 818.914')
    expect(wrapper.html()).not.toContain('m3.127 10.604')
  })
})
