import { mount } from '@vue/test-utils'
import { describe, expect, it } from 'vitest'
import PlatformIcon from '../PlatformIcon.vue'
import { ADMIN_PLATFORM_VALUES } from '../../../utils/platformOptions'

const imageRenderedPlatforms = new Set(['gemini', 'antigravity', 'kiro', 'zai'])

describe('PlatformIcon', () => {
  it('renders a downloaded brand icon for every supported admin platform', () => {
    for (const platform of ADMIN_PLATFORM_VALUES) {
      const wrapper = mount(PlatformIcon, {
        props: { platform, size: 'md' }
      })

      const maskIcon = wrapper.find('.platform-icon-mask')
      const imageIcon = wrapper.find('img.platform-icon-img')
      expect(maskIcon.exists() || imageIcon.exists(), platform).toBe(true)
      expect(wrapper.find('svg').exists(), platform).toBe(false)
    }
  })

  it('preserves full-color provider logos where a monochrome mask would collapse the mark', () => {
    for (const platform of imageRenderedPlatforms) {
      const wrapper = mount(PlatformIcon, {
        props: { platform: platform as never, size: 'md' }
      })

      expect(wrapper.find('img.platform-icon-img').exists(), platform).toBe(true)
      expect(wrapper.find('.platform-icon-mask').exists(), platform).toBe(false)
    }
  })

  it('keeps the generic fallback for unknown platforms', () => {
    const wrapper = mount(PlatformIcon, {
      props: { platform: 'unknown' as never, size: 'md' }
    })

    expect(wrapper.find('.platform-icon-mask').exists()).toBe(false)
    expect(wrapper.find('img.platform-icon-img').exists()).toBe(false)
    expect(wrapper.find('svg').exists()).toBe(true)
  })
})
