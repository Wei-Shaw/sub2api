import { mount } from '@vue/test-utils'
import { describe, expect, it } from 'vitest'
import PlatformIcon from '../PlatformIcon.vue'

describe('PlatformIcon', () => {
  it.each([
    { platform: 'anthropic', viewBox: '0 0 16 16' },
    { platform: 'openai', viewBox: '0 0 24 24' },
    { platform: 'grok', viewBox: '0 0 24 24' },
    { platform: 'gemini', viewBox: '0 0 24 24' },
    { platform: 'Anthropic', viewBox: '0 0 16 16' },
    { platform: 'OpenAI', viewBox: '0 0 24 24' },
    { platform: 'Grok', viewBox: '0 0 24 24' },
    { platform: 'unknown', viewBox: '0 0 24 24' },
  ])('renders correct svg for platform $platform', ({ platform, viewBox }) => {
    const wrapper = mount(PlatformIcon, { props: { platform, size: 'sm' } })
    const svg = wrapper.find('svg')
    expect(svg.exists()).toBe(true)
    expect(svg.attributes('viewBox')).toBe(viewBox)
  })
})
